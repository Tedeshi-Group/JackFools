package server // Пакет server содержит реализацию локального HTTP API.

import ( // Начинаем блок импортов.
	"encoding/json" // Кодируем ответы в JSON.
	"errors"        // Работаем с ошибками.
	"fmt"           // Форматируем пути файлов.
	"io"            // Читаем тело запроса.
	"log"           // Логируем события.
	"net/http"      // Реализуем HTTP сервер и обработчики.
	"os"            // Создаём директории и файлы.
	"regexp"        // Санитизируем имя действия.
	"strings"       // Нормализуем/сравниваем токен.
	"time"          // Таймауты сервера.

	"jackfools/client/internal/protocol" // Общие структуры протокола.
) // Закрываем блок импортов.

// QuestionAnswer — один правильный ответ на вопрос.
type QuestionAnswer struct { // Ответ на вопрос.
	Text  string `json:"text"`  // Текст ответа (точное совпадение с вариантом в игре).
	Index int    `json:"index"` // Информационный индекс (не используется для мэтчинга).
} // Конец QuestionAnswer.

// QuestionEntry — один вопрос в банке.
type QuestionEntry struct { // Запись вопроса.
	Prompt    string            `json:"prompt"`     // Текст вопроса (включая [i] теги).
	Answers   []QuestionAnswer  `json:"answers"`    // Правильные ответы.
	SeenCount int               `json:"seen_count"` // Сколько раз вопрос был замечен.
	LastSeen  int64             `json:"last_seen"`  // Unix timestamp последнего появления (ms).
} // Конец QuestionEntry.

// QuestionBank — банк вопросов.
type QuestionBank struct { // Банк вопросов.
	Questions []QuestionEntry `json:"questions"` // Массив вопросов.
} // Конец QuestionBank.

// QuestionStoreRequest — тело запроса POST /v1/questions.
type QuestionStoreRequest struct { // Запрос сохранения вопроса.
	Prompt  string            `json:"prompt"`  // Текст вопроса.
	Answers []QuestionAnswer  `json:"answers"` // Правильные ответы.
} // Конец QuestionStoreRequest.

type Config struct { // Конфигурация сервера.
	Addr         string        // Адрес вида 127.0.0.1:27124.
	Token        string        // Ожидаемый токен (значение X-JF-Token).
	ReadTimeout  time.Duration // Таймаут чтения запроса.
	WriteTimeout time.Duration // Таймаут записи ответа.
	IdleTimeout  time.Duration // Таймаут простоя соединения.
} // Конец Config.

func Run(cfg Config) error { // Запускает сервер и блокирует поток до остановки.
	if strings.TrimSpace(cfg.Addr) == "" { // Проверяем адрес.
		return errors.New("server: empty addr") // Возвращаем ошибку.
	} // Конец проверки.

	if strings.TrimSpace(cfg.Token) == "" { // Проверяем токен.
		return errors.New("server: empty token") // Возвращаем ошибку.
	} // Конец проверки.

	mux := http.NewServeMux() // Роутер стандартной библиотеки.

	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) { // Health-check.
		writeJSON(w, http.StatusOK, map[string]any{ // Отдаём JSON.
			"ok":      true,       // Признак работоспособности.
			"version": "dev",      // Версия (пока dev).
			"time":    time.Now(), // Серверное время.
		}) // Конец ответа.
	}) // Конец обработчика health.

	mux.HandleFunc("POST /v1/event", func(w http.ResponseWriter, r *http.Request) { // Приём событий от расширения.
		if !checkToken(r, cfg.Token) { // Проверяем токен.
			writeJSON(w, http.StatusUnauthorized, map[string]any{ // Отвечаем 401.
				"ok":    false,               // Указываем провал.
				"error": "unauthorized token", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец проверки токена.

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // Читаем тело, ограничивая размер (1 MiB).
		if err != nil {                                       // Если чтение не удалось.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":    false,           // Признак ошибки.
				"error": "read body failed", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец обработки ошибки чтения.

		var ev protocol.Event // Создаём структуру события.
		if err := json.Unmarshal(body, &ev); err != nil { // Парсим JSON.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":    false,          // Признак ошибки.
				"error": "invalid json", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец обработки ошибки JSON.

		if err := ev.Validate(); err != nil { // Валидируем событие.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":    false,              // Признак ошибки.
				"error": "invalid event",     // Сообщение.
				"detail": err.Error(),        // Детализация.
			}) // Конец ответа.
			return // Выходим.
		} // Конец валидации.

		log.Printf("event type=%s url=%s ts=%d payload_keys=%d", ev.Type, ev.URL, ev.TS, len(ev.Payload)) // Логируем.

		// Здесь позже можно подключить internal/hints и вернуть hint клиенту, если нужно.

		writeJSON(w, http.StatusOK, map[string]any{ // Возвращаем OK.
			"ok": true, // Признак успеха.
		}) // Конец ответа.
	}) // Конец обработчика event.

	mux.HandleFunc("POST /v1/recording", func(w http.ResponseWriter, r *http.Request) { // Приём записей от расширения.
		if !checkToken(r, cfg.Token) { // Проверяем токен.
			writeJSON(w, http.StatusUnauthorized, map[string]any{ // Отвечаем 401.
				"ok":    false,               // Указываем провал.
				"error": "unauthorized token", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец проверки токена.

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // Читаем тело, ограничивая размер (10 MiB).
		if err != nil {                                         // Если чтение не удалось.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":    false,             // Признак ошибки.
				"error": "read body failed", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец обработки ошибки чтения.

		var rec protocol.Recording // Создаём структуру записи.
		if err := json.Unmarshal(body, &rec); err != nil { // Парсим JSON.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":    false,          // Признак ошибки.
				"error": "invalid json", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец обработки ошибки JSON.

		if err := rec.Validate(); err != nil { // Валидируем запись.
			writeJSON(w, http.StatusBadRequest, map[string]any{ // Отвечаем 400.
				"ok":     false,             // Признак ошибки.
				"error":  "invalid recording", // Сообщение.
				"detail": err.Error(),        // Детализация.
			}) // Конец ответа.
			return // Выходим.
		} // Конец валидации.

		dir := "recordings" // Директория для сохранения.
		if err := os.MkdirAll(dir, 0o755); err != nil { // Создаём директорию если не существует.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ // Отвечаем 500.
				"ok":    false,                  // Признак ошибки.
				"error": "failed to create dir", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец создания директории.

		safeName := sanitizeActionName(rec.ActionName) // Санитизируем имя действия.
		filename := fmt.Sprintf("%d-%s.json", rec.StartedAt, safeName) // Формируем имя файла.
		filepath := fmt.Sprintf("%s/%s", dir, filename)                // Полный путь.

		data, err := json.MarshalIndent(rec, "", "  ") // Форматируем JSON с отступами.
		if err != nil {                                 // Если форматирование не удалось.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ // Отвечаем 500.
				"ok":    false,                    // Признак ошибки.
				"error": "failed to marshal json", // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец форматирования.

		if err := os.WriteFile(filepath, data, 0o644); err != nil { // Записываем файл.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ // Отвечаем 500.
				"ok":    false,                    // Признак ошибки.
				"error": "failed to write file",   // Сообщение.
			}) // Конец ответа.
			return // Выходим.
		} // Конец записи файла.

		log.Printf("recording saved: action=%s messages=%d path=%s", rec.ActionName, len(rec.Messages), filepath) // Логируем.

		writeJSON(w, http.StatusOK, map[string]any{ // Возвращаем OK.
			"ok":   true,        // Признак успеха.
			"path": filepath,    // Путь к сохранённому файлу.
		}) // Конец ответа.
	}) // Конец обработчика recording.

	// --- Question Bank endpoints ---

	const questionsFile = "questions.json" // Путь к файлу банка вопросов.

	mux.HandleFunc("GET /v1/questions", func(w http.ResponseWriter, r *http.Request) { // Возвращает полный банк вопросов.
		if !checkToken(r, cfg.Token) { // Проверяем токен.
			writeJSON(w, http.StatusUnauthorized, map[string]any{ "ok": false, "error": "unauthorized token" }) // 401.
			return // Выходим.
		} // Конец проверки токена.

		data, err := os.ReadFile(questionsFile) // Читаем файл банка вопросов.
		if err != nil { // Если файл не существует или не читается.
			writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "questions": []any{} }) // Возвращаем пустой банк.
			return // Выходим.
		} // Конец обработки ошибки.

		var bank QuestionBank // Парсим банк вопросов.
		if err := json.Unmarshal(data, &bank); err != nil { // Если JSON невалиден.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ "ok": false, "error": "invalid questions.json" }) // 500.
			return // Выходим.
		} // Конец парсинга.

		writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "questions": bank.Questions }) // Возвращаем банк.
	}) // Конец обработчика GET /v1/questions.

	mux.HandleFunc("POST /v1/questions", func(w http.ResponseWriter, r *http.Request) { // Добавляет/обновляет вопрос в банке.
		log.Printf("POST /v1/questions received") // Логируем получение запроса.
		if !checkToken(r, cfg.Token) { // Проверяем токен.
			writeJSON(w, http.StatusUnauthorized, map[string]any{ "ok": false, "error": "unauthorized token" }) // 401.
			return // Выходим.
		} // Конец проверки токена.

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // Читаем тело, ограничивая 1 MiB.
		if err != nil { // Если чтение не удалось.
			writeJSON(w, http.StatusBadRequest, map[string]any{ "ok": false, "error": "read body failed" }) // 400.
			return // Выходим.
		} // Конец обработки ошибки.

		var req QuestionStoreRequest // Парсим запрос.
		if err := json.Unmarshal(body, &req); err != nil { // Если JSON невалиден.
			writeJSON(w, http.StatusBadRequest, map[string]any{ "ok": false, "error": "invalid json" }) // 400.
			return // Выходим.
		} // Конец парсинга.

		if strings.TrimSpace(req.Prompt) == "" { // Если prompt пустой.
			writeJSON(w, http.StatusBadRequest, map[string]any{ "ok": false, "error": "prompt is required" }) // 400.
			return // Выходим.
		} // Конец проверки prompt.

		// Загружаем существующий банк.
		var bank QuestionBank // Банк вопросов.
		data, readErr := os.ReadFile(questionsFile) // Читаем файл.
		if readErr == nil { // Если файл существует.
			_ = json.Unmarshal(data, &bank) // Игнорируем ошибку парсинга (может быть пустой).
		} // Конец загрузки.

		// Ищем существующий вопрос по prompt.
		now := time.Now().UnixMilli() // Текущее время.
		found := false               // Флаг нахождения.
		for i, q := range bank.Questions { // Проходим по вопросам.
			if q.Prompt == req.Prompt { // Если prompt совпадает.
				bank.Questions[i].SeenCount++ // Увеличиваем счётчик.
				bank.Questions[i].LastSeen = now // Обновляем время.
				// Добавляем новые ответы, которых ещё нет.
				for _, newAns := range req.Answers { // Проходим по новым ответам.
					exists := false // Флаг существования.
					for _, existing := range bank.Questions[i].Answers { // Проверяем существующие.
						if existing.Text == newAns.Text { // Если ответ уже есть.
							exists = true // Устанавливаем флаг.
							break         // Прерываем.
						} // Конец проверки.
					} // Конец цикла.
					if !exists { // Если ответ новый.
						bank.Questions[i].Answers = append(bank.Questions[i].Answers, newAns) // Добавляем.
					} // Конец добавления.
				} // Конец цикла по новым ответам.
				found = true // Устанавливаем флаг.
				break        // Прерываем.
			} // Конец проверки prompt.
		} // Конец цикла.

		if !found { // Если вопрос новый.
			bank.Questions = append(bank.Questions, QuestionEntry{ // Добавляем.
				Prompt:    req.Prompt,    // Текст вопроса.
				Answers:   req.Answers,   // Ответы.
				SeenCount: 1,             // Первое появление.
				LastSeen:  now,           // Текущее время.
			}) // Конец добавления.
		} // Конец проверки.

		// Сохраняем банк.
		data, err = json.MarshalIndent(bank, "", "  ") // Форматируем JSON.
		if err != nil { // Если форматирование не удалось.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ "ok": false, "error": "marshal failed" }) // 500.
			return // Выходим.
		} // Конец форматирования.

		if err := os.WriteFile(questionsFile, data, 0o644); err != nil { // Записываем файл.
			writeJSON(w, http.StatusInternalServerError, map[string]any{ "ok": false, "error": "write failed" }) // 500.
			return // Выходим.
		} // Конец записи.

		log.Printf("question stored: prompt=%s answers=%d seen=%d", req.Prompt, len(req.Answers), func() int { // Логируем.
			for _, q := range bank.Questions { // Ищем вопрос.
				if q.Prompt == req.Prompt { return q.SeenCount } // Возвращаем счётчик.
			} // Конец поиска.
			return 0 // Не найден (не должно произойти).
		}()) // Конец логирования.

		writeJSON(w, http.StatusOK, map[string]any{ "ok": true }) // Возвращаем OK.
	}) // Конец обработчика POST /v1/questions.

	mux.HandleFunc("GET /v1/questions/lookup", func(w http.ResponseWriter, r *http.Request) { // Ищет вопрос по prompt.
		if !checkToken(r, cfg.Token) { // Проверяем токен.
			writeJSON(w, http.StatusUnauthorized, map[string]any{ "ok": false, "error": "unauthorized token" }) // 401.
			return // Выходим.
		} // Конец проверки токена.

		prompt := r.URL.Query().Get("prompt") // Получаем prompt из query string.
		if strings.TrimSpace(prompt) == "" { // Если prompt пустой.
			writeJSON(w, http.StatusBadRequest, map[string]any{ "ok": false, "error": "prompt parameter required" }) // 400.
			return // Выходим.
		} // Конец проверки prompt.

		data, err := os.ReadFile(questionsFile) // Читаем файл.
		if err != nil { // Если файл не существует.
			writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "found": false }) // Возвращаем not found.
			return // Выходим.
		} // Конец чтения.

		var bank QuestionBank // Парсим банк.
		if err := json.Unmarshal(data, &bank); err != nil { // Если JSON невалиден.
			writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "found": false }) // Возвращаем not found.
			return // Выходим.
		} // Конец парсинга.

		for _, q := range bank.Questions { // Ищем вопрос.
			if q.Prompt == prompt { // Если prompt совпадает.
				writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "found": true, "answers": q.Answers }) // Возвращаем ответы.
				return // Выходим.
			} // Конец проверки.
		} // Конец поиска.

		writeJSON(w, http.StatusOK, map[string]any{ "ok": true, "found": false }) // Вопрос не найден.
	}) // Конец обработчика GET /v1/questions/lookup.

	srv := &http.Server{ // Создаём HTTP сервер.
		Addr:         cfg.Addr,         // Адрес прослушивания.
		Handler:      mux,              // Роутер.
		ReadTimeout:  cfg.ReadTimeout,  // Таймаут чтения.
		WriteTimeout: cfg.WriteTimeout, // Таймаут записи.
		IdleTimeout:  cfg.IdleTimeout,  // Таймаут простоя.
	} // Конец конфигурации сервера.

	return srv.ListenAndServe() // Запускаем сервер (блокирует поток).
} // Конец Run.

func checkToken(r *http.Request, expected string) bool { // Проверка токена из заголовка.
	got := r.Header.Get("X-JF-Token") // Берём токен из заголовка.
	got = strings.TrimSpace(got)      // Убираем пробелы.
	return got != "" && got == expected // Сравниваем со значением из конфига.
} // Конец checkToken.

func writeJSON(w http.ResponseWriter, status int, v any) { // Утилита записи JSON ответа.
	w.Header().Set("Content-Type", "application/json; charset=utf-8") // Ставим content-type.
	w.WriteHeader(status)                                             // Ставим статус.
	enc := json.NewEncoder(w)                                         // Создаём энкодер.
	_ = enc.Encode(v)                                                 // Пишем JSON (ошибку игнорируем для простоты).
} // Конец writeJSON.

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]`) // Регулярка для санитизации имён.

func sanitizeActionName(name string) string { // Санитизирует имя действия для использования в имени файла.
	s := safeNameRe.ReplaceAllString(strings.TrimSpace(name), "_") // Заменяем небезопасные символы на подчёркивания.
	if len(s) > 64 {                                                // Если длиннее 64 символов.
		s = s[:64] // Обрезаем.
	} // Конец проверки длины.
	if s == "" { // Если после санитизации пустая строка.
		s = "unnamed" // Используем имя по умолчанию.
	} // Конец проверки пустоты.
	return s // Возвращаем результат.
} // Конец sanitizeActionName.




