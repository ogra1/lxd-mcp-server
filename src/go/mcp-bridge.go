package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"path/filepath"
)

type jsonRPCMessage struct {
	ID *json.RawMessage `json:"id"`
}

// Config structure for reading from config file
type Config struct {
	Port string `json:"port"`
}

func getConfigPort() string {
	// First check for config file
	configPathPrefix := os.Getenv("SNAP_DATA")
	configPath := filepath.Join(configPathPrefix, "config.json")
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err == nil {
			var config Config
			if err := json.Unmarshal(data, &config); err == nil {
				if config.Port != "" {
					return config.Port
				}
			}
		}
	}
	
	// Then check environment variable
	if envPort := os.Getenv("MCP_BRIDGE_PORT"); envPort != "" {
		return envPort
	}
	
	// Then check command line argument
	if len(os.Args) > 1 {
		if _, err := strconv.Atoi(os.Args[1]); err == nil {
			return os.Args[1]
		}
	}
	
	// Default port
	return "8005"
}

func main() {
	// Get port from various sources: config file > environment > command line > default
	port := getConfigPort()
	
	pathPrefix := os.Getenv("SNAP")
	toolPath := filepath.Join(pathPrefix, "bin", "lxd-mcp-server")
	cmd := exec.Command(toolPath)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	cmd.Stderr = os.Stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	var mu sync.Mutex
	pendingRequests := make(map[string]chan []byte)

	stdinWriter := bufio.NewWriter(stdinPipe)
	stdoutScanner := bufio.NewScanner(stdoutPipe)

	// Hintergrund-Thread: Liest kontinuierlich Pythons STDOUT
	go func() {
		for stdoutScanner.Scan() {
			line := stdoutScanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var msg jsonRPCMessage
			if err := json.Unmarshal(line, &msg); err == nil && msg.ID != nil {
				idStr := string(*msg.ID)

				mu.Lock()
				ch, exists := pendingRequests[idStr]
				if exists {
					resp := make([]byte, len(line))
					copy(resp, line)
					ch <- resp
					delete(pendingRequests, idStr)
				}
				mu.Unlock()
			}
		}
	}()

	http.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")

			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil || len(bodyBytes) == 0 {
				w.Write([]byte("{}"))
				return
			}

			// Prüfe, ob eine ID existiert
			var msg jsonRPCMessage
			_ = json.Unmarshal(bodyBytes, &msg)

			// FALL 1: Der Request hat KEINE ID (Notification)
			if msg.ID == nil {
				mu.Lock()
				_, _ = stdinWriter.Write(bodyBytes)
				_ = stdinWriter.WriteByte('\n')
				_ = stdinWriter.Flush()
				mu.Unlock()

				// WICHTIG FÜR LLAMA-UI: Sende 200 OK mit leerem JSON statt 204
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}

			// FALL 2: Der Request hat eine ID (Standard)
			idStr := string(*msg.ID)
			respChan := make(chan []byte, 1)

			mu.Lock()
			pendingRequests[idStr] = respChan
			_, _ = stdinWriter.Write(bodyBytes)
			_ = stdinWriter.WriteByte('\n')
			_ = stdinWriter.Flush()
			mu.Unlock()

			select {
			case responseBytes := <-respChan:
				w.Write(responseBytes)
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			case <-r.Context().Done():
				mu.Lock()
				delete(pendingRequests, idStr)
				mu.Unlock()
			}
			return
		}

		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {}
		}
	})

	// Print port info for debugging
	println("Starting MCP bridge on port " + port)
	_ = http.ListenAndServe("127.0.0.1:" + port, nil)
}

