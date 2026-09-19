package sink

// SinkCloser 合并 WriteLine/Close，便于 application 使用。
type SinkCloser interface {
	WriteLine(line []byte) error
	Close() error
}
