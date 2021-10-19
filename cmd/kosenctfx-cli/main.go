package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/theoremoon/kosenctfx-cli/data"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/xerrors"
)

func uploadFile(url, token, filename string, blob []byte) (string, error) {
	type Data struct {
		PresignedURL string `json:"presignedURL"`
		DownloadURL  string `json:"downloadURL"`
	}

	var data Data
	_, err := resty.New().SetAuthToken(token).R().
		SetBody(map[string]interface{}{"key": filename}).
		SetResult(&data).
		Post(url + "/admin/get-presigned-url")
	if err != nil {
		return "", xerrors.Errorf(": %w", err)
	}

	_, err = resty.New().R().
		SetBody(blob).
		Put(data.PresignedURL)
	if err != nil {
		return "", xerrors.Errorf(": %w", err)
	}

	return data.DownloadURL, nil
}

func setChallenge(url, token string, taskInfo data.TaskYaml) error {
	client := resty.New().SetAuthToken(token)
	_, err := client.R().
		SetBody(taskInfo).
		Post(url + "/admin/new-challenge")
	if err != nil {
		return xerrors.Errorf(": %w", err)
	}
	return nil
}

func makeDistfiles(dir, name string) ([]byte, error) {
	transform := fmt.Sprintf(" --transform 's:^\\./:./%s/:'", name)
	cmd := exec.Command("sh", "-c", "find . -type f | tar cz --files-from=- --to-stdout  --sort=name"+transform)
	cmd.Dir = dir
	tardata, err := cmd.Output()
	if err != nil {
		return nil, xerrors.Errorf(": %w", err)
	}
	return tardata, nil
}

func run() error {
	var url, token, dir, hashfile string
	flag.StringVar(&url, "url", "", "An endpoint of scoreserver")
	flag.StringVar(&token, "token", "", "An administrative token")
	flag.StringVar(&dir, "dir", "", "tasks directory")
	flag.StringVar(&hashfile, "hashfile", "", "hash file")
	flag.Usage = func() {
		fmt.Printf("Usage: %s\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if url == "" || token == "" || dir == "" {
		flag.Usage()
		return nil
	}
	url = strings.TrimSuffix(url, "/")

	hash_entries := make(map[string]string)

	if _, err := os.Stat(hashfile); err == nil {
		data, err := ioutil.ReadFile(hashfile)
		if err != nil {
			return xerrors.Errorf(": %w", err)
		}
		if err := json.Unmarshal(data, &hash_entries); err != nil {
			return xerrors.Errorf(": %w", err)
		}
	}

	targets := make(map[string]*data.TaskYaml)
	// walk tasks directory
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return xerrors.Errorf(": %w", err)
		}
		if info.Name() != "task.yml" {
			return nil
		}
		tasky, err := data.Load(path)
		if err != nil {
			return xerrors.Errorf(": %w", err)
		}

		// hash tableに乗っていない OR 更新されていたらtargetsに乗せる
		dirpath := filepath.Dir(path)
		h1, _ := dirhash.HashDir(dirpath, "", dirhash.Hash1)
		h2, exist := hash_entries[tasky.Name]
		if !exist || h1 != h2 {
			hash_entries[tasky.Name] = h1
			targets[dirpath] = tasky

		} else {
			log.Printf("[+] SKIP: %s\n", tasky.Name)
		}

		// このディレクトリは深堀りしない
		return filepath.SkipDir
	})
	if err != nil {
		return xerrors.Errorf(": %w", err)
	}

	for d, tasky := range targets {
		taskID := filepath.Base(d)
		attachments := make([]data.Attachment, 0, 10)
		err = func() error {
			distdir := filepath.Join(d, "distfiles")
			if _, err := os.Stat(distdir); err != nil {
				return nil
			}
			tardata, err := makeDistfiles(distdir, taskID)
			md5sum := md5.Sum(tardata)
			filename := fmt.Sprintf("%s_%s.tar.gz", taskID, hex.EncodeToString(md5sum[:]))
			dlUrl, err := uploadFile(url, token, filename, tardata)
			if err != nil {
				return xerrors.Errorf(": %w", err)
			}
			attachments = append(attachments, data.Attachment{
				URL:  dlUrl,
				Name: filename,
			})
			return nil
		}()
		if err != nil {
			return xerrors.Errorf(": %w", err)
		}

		err = func() error {
			rawDistdir := filepath.Join(d, "rawdistfiles")
			if _, err := os.Stat(rawDistdir); err != nil {
				return nil
			}
			err := filepath.Walk(rawDistdir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return xerrors.Errorf(": %w", err)
				}
				if info.IsDir() {
					return nil
				}
				blob, err := ioutil.ReadFile(path)
				if err != nil {
					return nil
				}

				dlUrl, err := uploadFile(url, token, info.Name(), blob)
				if err != nil {
					return xerrors.Errorf(": %w", err)
				}
				attachments = append(attachments, data.Attachment{
					URL:  dlUrl,
					Name: info.Name(),
				})
				return nil
			})
			if err != nil {
				return xerrors.Errorf(": %w", err)
			}
			return nil
		}()
		if err != nil {
			return xerrors.Errorf(": %w", err)
		}
		tasky.Attachments = attachments
		if err := setChallenge(url, token, *tasky); err != nil {
			return xerrors.Errorf(": %w", err)
		}

		log.Printf("[+] %s\n", tasky.Name)
	}

	// save
	hashb, err := json.Marshal(hash_entries)
	if err != nil {
		return xerrors.Errorf(": %w", err)
	}
	if err := ioutil.WriteFile(hashfile, hashb, 0755); err != nil {
		return xerrors.Errorf(": %w", err)
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%+v\n", err)
	}
}
