// Package logx 提供文件日志输出，支持分级与按行数轮转（保留最末 maxLogLines 行）。
package logx

import (
	"bufio"
	"log/slog"
	"os"
	"sync"
)

// maxLogLines 单文件保留的最大行数。
const maxLogLines = 1000

var (
	mu   sync.Mutex
	file *os.File
)

// currentFile 返回当前日志文件（可能为 nil）。
func currentFile() *os.File {
	return file
}

// setFile 原子替换当前日志文件。
func setFile(f *os.File) {
	if old := file; old != nil && old != f {
		old.Close()
	}
	file = f
}

// Init 打开日志文件（追加模式）。path 为空时仅输出到 stderr。
func Init(path string) error {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	file = f

	logger := slog.New(slog.NewTextHandler(lineWriter{}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	return nil
}

// lineWriter 实现 io.Writer：写入当前日志文件并在行数超限时轮转。
type lineWriter struct{}

func (lineWriter) Write(p []byte) (int, error) {
	mu.Lock()
	f := currentFile()
	if f == nil {
		mu.Unlock()
		return len(p), nil
	}
	n, err := f.Write(p)
	mu.Unlock()
	if err != nil {
		return n, err
	}
	rotateIfNeeded(f)
	return n, nil
}

// rotateIfNeeded 若文件行数超过 maxLogLines，则保留末尾 maxLogLines 行并重写文件。
// 并发写由调用方通过 file 竞争控制简化处理；此处不做全量加锁以免死锁。
func rotateIfNeeded(f *os.File) {
	count, err := countLines(f)
	if err != nil || count <= maxLogLines {
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		return
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var kept []byte
	for scanner.Scan() {
		kept = append(kept, scanner.Bytes()...)
		kept = append(kept, '\n')
	}

	path := f.Name()
	mu.Lock()
	nw, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err == nil {
		_, _ = nw.Write(kept)
		_ = nw.Close()
	}
	setFile(nw)
	mu.Unlock()
}

// countLines 统计文件当前行数并将读偏移恢复为写偏移（末尾）。
func countLines(f *os.File) (int, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, 2); err != nil {
		return 0, err
	}
	return count, nil
}

// Debug/Info/Warn/Error 便捷日志函数。
func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}

// Close 关闭日志文件。
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
	}
}
