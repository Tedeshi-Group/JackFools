package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"crypto/rand"      // Используем криптостойкий генератор случайных байт для токена.
	"encoding/hex"     // Нужен, чтобы представить токен в виде hex-строки.
	"flag"             // Парсим аргументы командной строки.
	"fmt"              // Печатаем сообщения пользователю.
	"log"              // Логируем события сервера/ошибки.
	"net"              // Валидируем/формируем адрес прослушивания.
	"os"               // Читаем аргументы и завершаем процесс с кодом.
	"time"             // Добавляем таймстемпы и таймауты.

	"jackfools/client/internal/server" // Наш локальный HTTP сервер.
) // Закрываем блок импортов.

// Serve реализует команду serve: запускает локальный API сервер.
// Принимает args — аргументы командной строки после подкоманды "serve".
func Serve(args []string) { // Реализация команды serve.
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)         // Создаём отдельный набор флагов.
	fs.SetOutput(os.Stderr)                                     // Ошибки парсинга пишем в stderr.
	port := fs.Int("port", 27124, "Local API port (localhost)") // Порт для localhost API.
	token := fs.String("token", "", "Auth token (hex). Empty = auto-generate") // Токен аутентификации.

	if err := fs.Parse(args); err != nil { // Парсим флаги.
		os.Exit(2) // Если флаги неверные — выходим.
	} // Конец обработки ошибки парсинга.

	if *port <= 0 || *port > 65535 { // Проверяем диапазон порта.
		log.Printf("invalid port: %d", *port) // Пишем в лог.
		os.Exit(2)                           // Выходим.
	} // Конец проверки порта.

	if *token == "" { // Если токен не задан.
		*token = mustGenerateToken(32) // Генерируем токен длиной 32 байта (64 hex-символа).
	} // Конец генерации токена.

	addr := net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", *port)) // Собираем адрес localhost:port.

	cfg := server.Config{ // Формируем конфиг сервера.
		Addr:         addr,           // Адрес прослушивания.
		Token:        *token,          // Токен для X-JF-Token.
		ReadTimeout:  5 * time.Second, // Таймаут чтения запроса.
		WriteTimeout: 5 * time.Second, // Таймаут записи ответа.
		IdleTimeout:  30 * time.Second, // Таймаут keep-alive.
	} // Конец конфига.

	log.Printf("local API listening on http://%s", addr) // Сообщаем адрес.
	log.Printf("X-JF-Token: %s", *token)                 // Печатаем токен (нужно ввести в options расширения).

	if err := server.Run(cfg); err != nil { // Запускаем сервер.
		log.Printf("server stopped with error: %v", err) // Логируем ошибку.
		os.Exit(1)                                       // Выходим с ошибкой.
	} // Конец обработки ошибки.
} // Конец Serve.

// mustGenerateToken генерирует криптостойкий токен заданной длины в байтах.
// Возвращает hex-строку (длина строки = nBytes * 2).
// Паникует, если генерация случайных байт невозможна (крайне редкий случай).
func mustGenerateToken(nBytes int) string { // Генератор токена.
	b := make([]byte, nBytes) // Создаём буфер байт.
	if _, err := rand.Read(b); err != nil { // Заполняем его случайными байтами.
		panic(err) // В случае невозможной ошибки — паникуем.
	} // Конец проверки ошибки.
	return hex.EncodeToString(b) // Возвращаем hex-строку.
} // Конец mustGenerateToken.

