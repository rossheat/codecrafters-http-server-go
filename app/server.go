package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	port           = ":4221"
	dataDir        = "/tmp/data/codecrafters.io/http-server-tester"
	maxRequestSize = 1024 * 1024 // 1MB
)

func main() {
	log.Println("Starting server on port", port)

	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to bind to port %s: %v", port, err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	req, err := parseRequest(conn)
	if err != nil {
		log.Printf("Error parsing request: %v", err)
		return
	}

	switch {
	case req.URL.Path == "/":
		handleRoot(conn)
	case req.URL.Path == "/user-agent":
		handleUserAgent(conn, req)
	case strings.HasPrefix(req.URL.Path, "/echo/"):
		handleEcho(conn, req)
	case strings.HasPrefix(req.URL.Path, "/files/"):
		handleFiles(conn, req)
	default:
		handleNotFound(conn)
	}
}

func parseRequest(conn net.Conn) (*http.Request, error) {
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return nil, err
	}

	// Limit the request body size
	req.Body = http.MaxBytesReader(nil, req.Body, maxRequestSize)

	return req, nil
}

func handleRoot(conn net.Conn) {
	sendResponse(conn, http.StatusOK, nil, nil)
}

func handleUserAgent(conn net.Conn, req *http.Request) {
	userAgent := req.Header.Get("User-Agent")
	sendResponse(conn, http.StatusOK, []byte(userAgent), map[string]string{"Content-Type": "text/plain"})
}

func handleEcho(conn net.Conn, req *http.Request) {
	parts := strings.SplitN(req.URL.Path, "/", 3)
	if len(parts) < 3 {
		handleNotFound(conn)
		return
	}

	content := []byte(parts[2])
	headers := map[string]string{"Content-Type": "text/plain"}

	if acceptsGzip(req) {
		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		if _, err := gzipWriter.Write(content); err != nil {
			log.Printf("Error compressing content: %v", err)
			sendResponse(conn, http.StatusInternalServerError, nil, nil)
			return
		}
		gzipWriter.Close()

		content = buf.Bytes()
		headers["Content-Encoding"] = "gzip"
	}

	sendResponse(conn, http.StatusOK, content, headers)
}

func handleFiles(conn net.Conn, req *http.Request) {
	filename := filepath.Base(req.URL.Path)
	filePath := filepath.Join(dataDir, filename)

	switch req.Method {
	case http.MethodGet:
		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				handleNotFound(conn)
			} else {
				log.Printf("Error reading file: %v", err)
				sendResponse(conn, http.StatusInternalServerError, nil, nil)
			}
			return
		}
		sendResponse(conn, http.StatusOK, content, map[string]string{"Content-Type": "application/octet-stream"})

	case http.MethodPost:
		content, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			sendResponse(conn, http.StatusInternalServerError, nil, nil)
			return
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			log.Printf("Error creating directory: %v", err)
			sendResponse(conn, http.StatusInternalServerError, nil, nil)
			return
		}

		if err := os.WriteFile(filePath, content, 0666); err != nil {
			log.Printf("Error writing file: %v", err)
			sendResponse(conn, http.StatusInternalServerError, nil, nil)
			return
		}

		sendResponse(conn, http.StatusCreated, nil, nil)

	default:
		sendResponse(conn, http.StatusMethodNotAllowed, nil, nil)
	}
}

func handleNotFound(conn net.Conn) {
	sendResponse(conn, http.StatusNotFound, nil, nil)
}

func sendResponse(conn net.Conn, status int, content []byte, headers map[string]string) {
	resp := &http.Response{
		Status:     http.StatusText(status),
		StatusCode: status,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(content)),
	}

	for k, v := range headers {
		resp.Header.Set(k, v)
	}

	if content != nil {
		resp.ContentLength = int64(len(content))
	}

	if err := resp.Write(conn); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

func acceptsGzip(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept-Encoding"), "gzip")
}
