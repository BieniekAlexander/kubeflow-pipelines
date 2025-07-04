// Copyright 2018 The Kubeflow Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"bytes"
	"context"
	"path"
	"regexp"

	"github.com/kubeflow/pipelines/backend/src/common/util"
	minio "github.com/minio/minio-go/v7"
	"sigs.k8s.io/yaml"
)

const (
	multipartDefaultSize = -1
)

// Interface for object store.
type ObjectStoreInterface interface {
	AddFile(ctx context.Context, template []byte, filePath string) error
	DeleteFile(ctx context.Context, filePath string) error
	GetFile(ctx context.Context, filePath string) ([]byte, error)
	AddAsYamlFile(ctx context.Context, o interface{}, filePath string) error
	GetFromYamlFile(ctx context.Context, o interface{}, filePath string) error
	GetPipelineKey(pipelineId string) string
}

// Managing pipeline using Minio.
type MinioObjectStore struct {
	minioClient      MinioClientInterface
	bucketName       string
	baseFolder       string
	disableMultipart bool
}

// GetPipelineKey adds the configured base folder to pipeline id.
func (m *MinioObjectStore) GetPipelineKey(pipelineID string) string {
	return path.Join(m.baseFolder, pipelineID)
}

func (m *MinioObjectStore) AddFile(ctx context.Context, file []byte, filePath string) error {
	var parts int64

	if m.disableMultipart {
		parts = int64(len(file))
	} else {
		parts = multipartDefaultSize
	}

	_, err := m.minioClient.PutObject(
		ctx,
		m.bucketName, filePath, bytes.NewReader(file),
		parts, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return util.NewInternalServerError(err, "Failed to store file %v", filePath)
	}
	return nil
}

func (m *MinioObjectStore) DeleteFile(ctx context.Context, filePath string) error {
	err := m.minioClient.DeleteObject(ctx, m.bucketName, filePath)
	if err != nil {
		return util.NewInternalServerError(err, "Failed to delete file %v", filePath)
	}
	return nil
}

func (m *MinioObjectStore) GetFile(ctx context.Context, filePath string) ([]byte, error) {
	reader, err := m.minioClient.GetObject(ctx, m.bucketName, filePath, minio.GetObjectOptions{})
	if err != nil {
		return nil, util.NewInternalServerError(err, "Failed to get file %v", filePath)
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(reader)

	bytes := buf.Bytes()

	// Remove single part signature if exists
	if m.disableMultipart {
		re := regexp.MustCompile(`\w+;chunk-signature=\w+`)
		bytes = []byte(re.ReplaceAllString(string(bytes), ""))
	}

	return bytes, nil
}

func (m *MinioObjectStore) AddAsYamlFile(ctx context.Context, o interface{}, filePath string) error {
	bytes, err := yaml.Marshal(o)
	if err != nil {
		return util.NewInternalServerError(err, "Failed to marshal file %v: %v", filePath, err.Error())
	}
	err = m.AddFile(ctx, bytes, filePath)
	if err != nil {
		return util.Wrap(err, "Failed to add a yaml file")
	}
	return nil
}

func (m *MinioObjectStore) GetFromYamlFile(ctx context.Context, o interface{}, filePath string) error {
	bytes, err := m.GetFile(ctx, filePath)
	if err != nil {
		return util.Wrap(err, "Failed to read from a yaml file")
	}
	err = yaml.Unmarshal(bytes, o)
	if err != nil {
		return util.NewInternalServerError(err, "Failed to unmarshal file %v: %v", filePath, err.Error())
	}
	return nil
}

func NewMinioObjectStore(minioClient MinioClientInterface, bucketName string, baseFolder string, disableMultipart bool) *MinioObjectStore {
	return &MinioObjectStore{minioClient: minioClient, bucketName: bucketName, baseFolder: baseFolder, disableMultipart: disableMultipart}
}
