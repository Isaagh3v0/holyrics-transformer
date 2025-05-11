package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	mutex      sync.Mutex
	lastText   string
	lastHeader string
	lastType   string
)

func startTextPolling(_ interface{}, lyricsURL, lyricsPort string) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var lastErrorTime time.Time
	var isConnected bool

	for range ticker.C {
		url := fmt.Sprintf("http://%s:%s/view/text.json", lyricsURL, lyricsPort)
		resp, err := http.Get(url)
		if err != nil {
			// Логируем ошибку только раз в 5 секунд
			if time.Since(lastErrorTime) > 5*time.Second {
				if !isConnected {
					log.Printf("Попытка подключения к REST серверу...")
				} else {
					log.Printf("Потеряно соединение с REST сервером: %v", err)
				}
				lastErrorTime = time.Now()
			}
			isConnected = false
			continue
		}

		if !isConnected {
			log.Printf("Успешное подключение к REST серверу")
			isConnected = true
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Ошибка при чтении ответа: %v", err)
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			log.Printf("Ошибка при разборе JSON: %v", err)
			continue
		}

		mapData, ok := data["map"].(map[string]interface{})
		if !ok {
			log.Println("Неверный формат данных")
			continue
		}

		text, _ := mapData["text"].(string)
		header, _ := mapData["header"].(string)
		textType, _ := mapData["type"].(string)
		textType = strings.ToUpper(textType)

		cleanedContent := cleanTextToArray(text, textType)
		cleanedText := strings.Join(cleanedContent, "\n")

		mutex.Lock()
		if cleanedText != lastText {
			lastText = cleanedText
			lastHeader = header
			lastType = textType

			payload := map[string]interface{}{
				"type":    lastType,
				"header":  lastHeader,
				"content": cleanedContent,
			}

			// Рассылка всем WebSocket-клиентам
			clientsMu.Lock()
			for conn := range clients {
				err := conn.WriteJSON(payload)
				if err != nil {
					Error(fmt.Sprintf("Ошибка отправки сообщения клиенту %s: %v", conn.RemoteAddr().String(), err))
					conn.Close()
					delete(clients, conn)
				}
			}
			clientsMu.Unlock()

			log.Println("Отправлен обновлённый текст")
		}
		mutex.Unlock()
	}
}

func cleanTextToArray(text, textType string) []string {
	re := regexp.MustCompile(`<span[^>]*id=['"]text-force-update_\d+['"][^>]*>.*?</span>`)
	text = re.ReplaceAllString(text, "")

	reHTML := regexp.MustCompile(`<[^>]+>`)
	cleanText := reHTML.ReplaceAllString(text, "")

	cleanText = strings.ReplaceAll(cleanText, " ", " ")
	cleanText = strings.ReplaceAll(cleanText, "&nbsp;", " ")
	cleanText = strings.ReplaceAll(cleanText, "&amp;", "&")
	cleanText = strings.ReplaceAll(cleanText, "&lt;", "<")
	cleanText = strings.ReplaceAll(cleanText, "&gt;", ">")
	cleanText = strings.ReplaceAll(cleanText, "&quot;", "\"")
	cleanText = strings.ReplaceAll(cleanText, "&#39;", "'")

	reArtifact := regexp.MustCompile(`\(SYN - [A-Z0-9]+\)`)
	cleanText = reArtifact.ReplaceAllString(cleanText, "")

	cleanText = strings.TrimSpace(cleanText)

	var lines []string
	if textType == "BIBLE" {
		lines = processBibleText(cleanText)
	} else {
		lines = strings.Split(cleanText, "\n")
		lines = filterEmptyLines(lines)
	}

	// Окончательная очистка каждой строки
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return lines
}

func filterEmptyLines(lines []string) []string {
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func processBibleText(text string) []string {
	plainText := text
	plainText = strings.ReplaceAll(plainText, " ", " ")
	plainText = strings.TrimSpace(plainText)
	plainText = strings.TrimSuffix(plainText, ")")
	parts := strings.Split(plainText, ". ")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
