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

	for range ticker.C {
		url := fmt.Sprintf("http://%s:%s/view/text.json", lyricsURL, lyricsPort)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Ошибка при получении текста: %v", err)
			continue
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

		mutex.Lock()
		if text != lastText {
			lastText = text
			lastHeader = header
			lastType = strings.ToUpper(textType)

			content := cleanTextToArray(text, lastType)
			payload := map[string]interface{}{
				"type":    lastType,
				"header":  lastHeader,
				"content": content,
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

// cleanTextToArray очищает текст от HTML-тегов и &nbsp;, возвращает массив строк
func cleanTextToArray(text, textType string) []string {
	// Сначала удаляем span с id='text-force-update' со всеми возможными вариациями
	re := regexp.MustCompile(`<span\s+style=['"]visibility:hidden;display:none['"]?\s+id=['"]text-force-update_\d+['"]?\s*>\s*</span>`)
	text = re.ReplaceAllString(text, "")

	// Более общий подход для удаления HTML-тегов
	reHTML := regexp.MustCompile(`<[^>]+>`)
	cleanText := reHTML.ReplaceAllString(text, "")

	// Замена &nbsp; и других специальных символов
	cleanText = strings.ReplaceAll(cleanText, " ", " ")
	cleanText = strings.ReplaceAll(cleanText, "&nbsp;", " ")

	// Удаление специфических артефактов, например, (SYN - AZJ08)
	reArtifact := regexp.MustCompile(`\(SYN - [A-Z0-9]+\)`)
	cleanText = reArtifact.ReplaceAllString(cleanText, "")

	// Удаление лишних пробелов
	cleanText = strings.TrimSpace(cleanText)

	// Обработка в зависимости от типа
	var lines []string
	if textType == "BIBLE" {
		lines = processBibleText(cleanText)
	} else {
		// Разделение по \n для MUSIC
		lines = strings.Split(cleanText, "\n")
		lines = filterEmptyLines(lines)
	}

	// Окончательная очистка каждой строки
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return lines
}

// filterEmptyLines удаляет пустые строки
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
