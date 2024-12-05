package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"log"
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

// 定义一个客户端结构体
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// WebSocket 连接的升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 存储连接的客户端
var clients = make(map[*Client]bool)

// 广播消息给所有连接的客户端
func broadcastMessage(message []byte) {
	for client := range clients {
		select {
		case client.send <- message:
		default:
			close(client.send)
			delete(clients, client)
		}
	}
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

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// 升级 HTTP 请求为 WebSocket 连接
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()

		client := &Client{conn: conn, send: make(chan []byte)}
		clients[client] = true
		defer delete(clients, client)

		// 启动一个 goroutine 来处理客户端的消息发送
		go func() {
			for message := range client.send {
				err := conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Println(err)
					return
				}
			}
		}()

		// 接收消息并广播
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println(err)
				return
			}
			// 广播接收到的消息
			broadcastMessage(message)
		}
	})

	fmt.Println("Starting server on :3000")
	http.ListenAndServe(":3000", nil)
}
