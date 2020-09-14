// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/seekjoiner"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestDirs(t *testing.T) {
	var (
		dirUploadResource    = "/dirs"
		fileDownloadResource = func(addr string) string { return "/files/" + addr }
		storer               = mock.NewStorer()
		mockStatestore       = statestore.NewStateStore()
		logger               = logging.New(ioutil.Discard, 0)
		client               = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(mockStatestore, logger),
			Logger: logging.New(ioutil.Discard, 5),
		})
	)

	t.Run("empty request body", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(nil)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "could not validate request",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
		)
	})

	t.Run("non tar file", func(t *testing.T) {
		file := bytes.NewReader([]byte("some data"))

		jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusInternalServerError,
			jsonhttptest.WithRequestBody(file),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "could not store dir",
				Code:    http.StatusInternalServerError,
			}),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
		)
	})

	t.Run("wrong content type", func(t *testing.T) {
		tarReader := tarFiles(t, []f{{
			data: []byte("some data"),
			name: "binary-file",
		}})

		// submit valid tar, but with wrong content-type
		jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(tarReader),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "could not validate request",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithRequestHeader("Content-Type", "other"),
		)
	})

	// valid tars
	for _, tc := range []struct {
		name                string
		wantIndexFilename   string
		indexFilenameOption jsonhttptest.Option
		files               []f // files in dir for test case
	}{
		{
			name: "non-nested files without extension",
			files: []f{
				{
					data:      []byte("first file data"),
					name:      "file1",
					dir:       "",
					reference: swarm.MustParseHexAddress("3c07cd2cf5c46208d69d554b038f4dce203f53ac02cb8a313a0fe1e3fe6cc3cf"),
					header: http.Header{
						"Content-Type": {""},
					},
				},
				{
					data:      []byte("second file data"),
					name:      "file2",
					dir:       "",
					reference: swarm.MustParseHexAddress("47e1a2a8f16e02da187fac791d57e6794f3e9b5d2400edd00235da749ad36683"),
					header: http.Header{
						"Content-Type": {""},
					},
				},
			},
		},
		{
			name: "nested files with extension",
			files: []f{
				{
					data:      []byte("robots text"),
					name:      "robots.txt",
					dir:       "",
					reference: swarm.MustParseHexAddress("17b96d0a800edca59aaf7e40c6053f7c4c0fb80dd2eb3f8663d51876bf350b12"),
					header: http.Header{
						"Content-Type": {"text/plain; charset=utf-8"},
					},
				},
				{
					data:      []byte("image 1"),
					name:      "1.png",
					dir:       "img",
					reference: swarm.MustParseHexAddress("3c1b3fc640e67f0595d9c1db23f10c7a2b0bdc9843b0e27c53e2ac2a2d6c4674"),
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
				{
					data:      []byte("image 2"),
					name:      "2.png",
					dir:       "img",
					reference: swarm.MustParseHexAddress("b234ea7954cab7b2ccc5e07fe8487e932df11b2275db6b55afcbb7bad0be73fb"),
					header: http.Header{
						"Content-Type": {"image/png"},
					},
				},
			},
		},
		{
			name: "no index filename",
			files: []f{
				{
					data:      []byte("<h1>Swarm"),
					name:      "index.html",
					dir:       "",
					reference: swarm.MustParseHexAddress("bcb1bfe15c36f1a529a241f4d0c593e5648aa6d40859790894c6facb41a6ef28"),
					header: http.Header{
						"Content-Type": {"text/html; charset=utf-8"},
					},
				},
			},
		},
		{
			name:                "explicit index filename",
			wantIndexFilename:   "index.html",
			indexFilenameOption: jsonhttptest.WithRequestHeader(api.SwarmIndextHeader, "index.html"),
			files: []f{
				{
					data:      []byte("<h1>Swarm"),
					name:      "index.html",
					dir:       "",
					reference: swarm.MustParseHexAddress("bcb1bfe15c36f1a529a241f4d0c593e5648aa6d40859790894c6facb41a6ef28"),
					header: http.Header{
						"Content-Type": {"text/html; charset=utf-8"},
					},
				},
			},
		},
		{
			name:                "nested index filename",
			wantIndexFilename:   "index.html",
			indexFilenameOption: jsonhttptest.WithRequestHeader(api.SwarmIndextHeader, "index.html"),
			files: []f{
				{
					data:      []byte("<h1>Swarm"),
					name:      "index.html",
					dir:       "dir",
					reference: swarm.MustParseHexAddress("bcb1bfe15c36f1a529a241f4d0c593e5648aa6d40859790894c6facb41a6ef28"),
					header: http.Header{
						"Content-Type": {"text/html; charset=utf-8"},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// tar all the test case files
			tarReader := tarFiles(t, tc.files)

			var respBytes []byte

			options := []jsonhttptest.Option{
				jsonhttptest.WithRequestBody(tarReader),
				jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
				jsonhttptest.WithPutResponseBody(&respBytes),
			}
			if tc.indexFilenameOption != nil {
				options = append(options, tc.indexFilenameOption)
			}

			// verify directory tar upload response
			jsonhttptest.Request(t, client, http.MethodPost, dirUploadResource, http.StatusOK, options...)

			read := bytes.NewReader(respBytes)

			// get the reference as everytime it will change because of random encryption key
			var resp api.FileUploadResponse
			err := json.NewDecoder(read).Decode(&resp)
			if err != nil {
				t.Fatal(err)
			}

			// NOTE: reference will be different each time, due to manifest randomness

			if resp.Reference.String() == "" {
				t.Fatalf("expected file reference, did not got any")
			}

			// read manifest metadata
			j := seekjoiner.NewSimpleJoiner(storer)

			buf := bytes.NewBuffer(nil)
			_, err = file.JoinReadAll(context.Background(), j, resp.Reference, buf)
			if err != nil {
				t.Fatal(err)
			}
			e := &entry.Entry{}
			err = e.UnmarshalBinary(buf.Bytes())
			if err != nil {
				t.Fatal(err)
			}

			// verify manifest content
			verifyManifest, err := manifest.NewManifestReference(
				context.Background(),
				manifest.DefaultManifestType,
				e.Reference(),
				false,
				storer,
			)
			if err != nil {
				t.Fatal(err)
			}

			validateFile := func(t *testing.T, file f, filePath string) {
				t.Helper()

				entry, err := verifyManifest.Lookup(filePath)
				if err != nil {
					t.Fatal(err)
				}

				fileReference := entry.Reference()

				if !bytes.Equal(file.reference.Bytes(), fileReference.Bytes()) {
					t.Fatalf("expected file reference to match %s, got %s", file.reference, fileReference)
				}

				jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(fileReference.String()), http.StatusOK,
					jsonhttptest.WithExpectedResponse(file.data),
					jsonhttptest.WithRequestHeader("Content-Type", file.header.Get("Content-Type")),
				)
			}

			// check if each file can be located and read
			for _, file := range tc.files {
				validateFile(t, file, path.Join(file.dir, file.name))

				// if there is an index filename to be tested
				// try to download it using only the directory as the path
				if file.name == tc.wantIndexFilename {
					validateFile(t, file, file.dir)
				}
			}

		})
	}
}

// tarFiles receives an array of test case files and creates a new tar with those files as a collection
// it returns a bytes.Buffer which can be used to read the created tar
func tarFiles(t *testing.T, files []f) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, file := range files {
		// create tar header and write it
		hdr := &tar.Header{
			Name: path.Join(file.dir, file.name),
			Mode: 0600,
			Size: int64(len(file.data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}

		// write the file data to the tar
		if _, err := tw.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}

	// finally close the tar writer
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	return &buf
}

// struct for dir files for test cases
type f struct {
	data      []byte
	name      string
	dir       string
	reference swarm.Address
	header    http.Header
}