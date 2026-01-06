package server // Пакет server содержит реализацию локального HTTP API.

import ( // Начинаем блок импортов.
	"encoding/json" // Кодируем ответы в JSON.
	"errors"        // Работаем с ошибками.
	"io"            // Читаем тело запроса.
	"log"           // Логируем события.
	"net/http"      // Реализуем HTTP сервер и обработчики.
	"strings"       // Нормализуем/сравниваем токен.
	"time"          // Таймауты сервера.

	"jackfools/client/internal/protocol" // Общие структуры протокола.
) // Закрываем блок импортов.

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



