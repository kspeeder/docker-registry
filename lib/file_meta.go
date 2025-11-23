package lib

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
)

type FileMeta struct {
	Size      int64 `json:"size"`
	ChunkSize int64 `json:"chunk_size"`
	ModTime   int64 `json:"mod_time"`
}

func ReadFileMeta(cachePath string) (*FileMeta, error) {
	b, err := ioutil.ReadFile(filepath.Join(cachePath, "file_meta.json"))
	if err != nil {
		return nil, err
	}
	meta := &FileMeta{}
	err = json.Unmarshal(b, &meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func SaveFileMeta(cachePath string, meta *FileMeta) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(cachePath, "file_meta.json"), b, 0755)
}

func IsMetaFileEqual(a, b *FileMeta) bool {
	return a.Size == b.Size && a.ChunkSize == b.ChunkSize
}

func IsMetaValid(cachePath string, chunkSize, totalSize int64) bool {
	meta, err := ReadFileMeta(cachePath)
	if err != nil {
		return false
	}
	return meta.ChunkSize == chunkSize && meta.Size == totalSize
}
