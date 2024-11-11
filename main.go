package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const uploadDir = "/tmp"

type MoveFileRequest struct {
	FileName string `json:"fileName"`
}

func main() {
	// 确保上传目录存在
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		fmt.Println("Error creating upload directory:", err)
		return
	}

	// 上传文件块
	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		// 获取上传的文件块
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Failed to get file from request", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// 获取文件名和偏移量
		fileName := r.FormValue("fileName")
		offsetStr := r.FormValue("offset")
		offset, err := strconv.ParseInt(offsetStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid offset", http.StatusBadRequest)
			return
		}

		// 打开或创建目标文件
		filePath := filepath.Join(uploadDir, fileName)
		out, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			http.Error(w, "Failed to open file", http.StatusInternalServerError)
			return
		}
		defer out.Close()

		// 将偏移量设置为文件的当前位置
		_, err = out.Seek(offset, io.SeekStart)
		if err != nil {
			http.Error(w, "Failed to seek in file", http.StatusInternalServerError)
			return
		}

		// 将上传的块写入文件
		_, err = io.Copy(out, file)
		if err != nil {
			http.Error(w, "Failed to write file", http.StatusInternalServerError)
			return
		}

		fmt.Fprintln(w, "Chunk uploaded successfully")
	})

	http.HandleFunc("/move-file", func(w http.ResponseWriter, r *http.Request) {
		var req MoveFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		destPath := "/usr/share/nginx/html/temp/" + req.FileName // 目标文件路径

		// 执行文件移动
		cmd := exec.Command("mv", "/tmp/"+req.FileName, destPath)

		// 运行命令并捕获错误
		if err := cmd.Run(); err != nil {
			http.Error(w, "Failed to move file "+err.Error(), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "File moved successfully")
	})

	// 查询上传进度
	http.HandleFunc("/upload/progress", func(w http.ResponseWriter, r *http.Request) {
		fileName := r.URL.Query().Get("fileName")
		if fileName == "" {
			http.Error(w, "Filename is required", http.StatusBadRequest)
			return
		}

		filePath := filepath.Join(uploadDir, fileName)
		fileInfo, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"uploadedBytes": 0}`))
			return
		} else if err != nil {
			http.Error(w, "Failed to get file info", http.StatusInternalServerError)
			return
		}

		uploadedBytes := fileInfo.Size()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"uploadedBytes": %d}`, uploadedBytes)))
	})
	http.Handle("/", http.FileServer(http.Dir("./html"))) // 提供静态文件服务

	fmt.Println("Starting server on :3000")
	http.ListenAndServe(":3000", nil)
}
