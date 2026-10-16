package teldrive

import (
	"io"

	"github.com/alist-org/alist/v3/internal/model"
)

// streamSectionReader 是 OpenList v4 internal/stream.StreamSectionReader 的等价替代。
//
// alist v3 的 internal/stream 包没有 StreamSectionReader 组件（那是 OpenList 新增的），
// 但它提供了同样够用的能力：FileStreamer.CacheFullInTempFile() 会把上传流完整落到
// 一个临时文件上（model.File 实现了 io.ReaderAt），于是每个分片都可以用
// io.NewSectionReader 独立切出来读。
//
// 与 OpenList 版的行为差异：OpenList 走混合缓存（边缓存边读，省磁盘）；
// 这里先把整个文件缓存到临时文件再切分，多占一份临时磁盘空间，但语义一致、实现更稳。
type streamSectionReader struct {
	cache model.File
}

func newStreamSectionReader(file model.FileStreamer) (*streamSectionReader, error) {
	cache, err := file.CacheFullInTempFile()
	if err != nil {
		return nil, err
	}
	return &streamSectionReader{cache: cache}, nil
}

// GetSectionReader 取出 [off, off+length) 区间的读取器。
// 线程不安全性仅为保持与 OpenList 相同的调用约定（调用方自行加锁）；
// 本实现每个读取器互相独立，本身无共享可变状态。
func (s *streamSectionReader) GetSectionReader(off, length int64) (io.ReadSeeker, error) {
	return io.NewSectionReader(s.cache, off, length), nil
}

// FreeSectionReader 线程安全。本实现无缓冲块需要回收。
func (s *streamSectionReader) FreeSectionReader(sr io.ReadSeeker) {}

// DiscardSection 线程不安全。本实现基于完整缓存文件，无需丢弃。
func (s *streamSectionReader) DiscardSection(off int64, length int64) error { return nil }
