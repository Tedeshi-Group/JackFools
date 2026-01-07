package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"context"       // Отменяем запросы при нахождении актуального хоста.
	"crypto/rand"   // Генерируем криптографически стойкие случайные числа для имён.
	"encoding/json" // Парсим JSON с хостами.
	"fmt"           // Печатаем сообщения пользователю.
	"io"            // Читаем тело ответа HTTP.
	"log"           // Логируем события сервера/ошибки.
	"net/http"      // Отправляем HTTP запросы к хостам.
	"net/url"       // Работаем с URL параметрами.
	"os"            // Читаем аргументы и завершаем процесс с кодом.
	"strings"       // Работаем со строками для валидации.
	"sync"          // Синхронизируем горутины.
	"time"          // Устанавливаем таймауты для запросов.
	"unicode"       // Проверяем, является ли символ буквой.

	"github.com/google/uuid"       // Генерируем UUID для user-id.
	"github.com/gorilla/websocket" // Работаем с WebSocket подключениями.
) // Закрываем блок импортов.

// hostsResponse представляет структуру JSON ответа с хостами.
// Может быть либо массивом строк, либо объектом с полем hosts.
type hostsResponse struct { // Структура для парсинга JSON.
	Hosts []string `json:"hosts"` // Поле hosts, если JSON - объект.
} // Конец hostsResponse.

// RoomInfo содержит информацию о комнате игры из API ответа.
type RoomInfo struct { // Структура для информации о комнате.
	AppID             string `json:"appId"`             // ID приложения игры.
	AppTag            string `json:"appTag"`            // Тег приложения игры.
	AudienceEnabled   bool   `json:"audienceEnabled"`   // Включена ли аудитория.
	Code              string `json:"code"`              // Код комнаты.
	Host              string `json:"host"`              // Хост для подключения.
	AudienceHost      string `json:"audienceHost"`      // Хост для аудитории.
	Locked            bool   `json:"locked"`            // Заблокирована ли комната.
	Full              bool   `json:"full"`              // Полна ли комната.
	MaxPlayers        int    `json:"maxPlayers"`        // Максимальное количество игроков.
	MinPlayers        int    `json:"minPlayers"`        // Минимальное количество игроков.
	ModerationEnabled bool   `json:"moderationEnabled"` // Включена ли модерация.
	PasswordRequired  bool   `json:"passwordRequired"`  // Требуется ли пароль.
	TwitchLocked      bool   `json:"twitchLocked"`      // Заблокирована ли для Twitch.
	Locale            string `json:"locale"`            // Локаль комнаты.
	Keepalive         bool   `json:"keepalive"`         // Включен ли keepalive.
	ControllerBranch  string `json:"controllerBranch"`  // Ветка контроллера.
} // Конец RoomInfo.

// RoomResponse представляет полный JSON ответ API о комнате.
type RoomResponse struct { // Структура для ответа API.
	OK   bool     `json:"ok"`   // Флаг успешности ответа.
	Body RoomInfo `json:"body"` // Тело ответа с информацией о комнате.
} // Конец RoomResponse.

// fetchHosts загружает список хостов из JSON файла по URL.
// Возвращает слайс строк с хостами или ошибку.
func fetchHosts(url string) ([]string, error) { // Функция загрузки хостов.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Создаём контекст с таймаутом 10 секунд для загрузки списка хостов.
	defer cancel()                                                           // Отменяем контекст при выходе из функции.

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Создаём HTTP GET запрос с контекстом.
	if err != nil {                                              // Если не удалось создать запрос.
		return nil, fmt.Errorf("failed to create request: %w", err) // Возвращаем ошибку.
	} // Конец проверки создания запроса.

	client := &http.Client{}    // Создаём HTTP клиент.
	resp, err := client.Do(req) // Выполняем запрос.
	if err != nil {             // Если запрос не удался.
		return nil, fmt.Errorf("failed to fetch hosts: %w", err) // Возвращаем ошибку.
	} // Конец проверки выполнения запроса.
	defer resp.Body.Close() // Закрываем тело ответа при выходе из функции.

	if resp.StatusCode != http.StatusOK { // Если статус ответа не 200.
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode) // Возвращаем ошибку.
	} // Конец проверки статуса.

	body, err := io.ReadAll(resp.Body) // Читаем всё тело ответа.
	if err != nil {                    // Если не удалось прочитать тело.
		return nil, fmt.Errorf("failed to read response body: %w", err) // Возвращаем ошибку.
	} // Конец проверки чтения тела.

	// Пробуем распарсить как объект с полем hosts.
	var hostsObj hostsResponse                                                         // Создаём переменную для объекта с хостами.
	if err := json.Unmarshal(body, &hostsObj); err == nil && len(hostsObj.Hosts) > 0 { // Если удалось распарсить как объект и хосты есть.
		return hostsObj.Hosts, nil // Возвращаем хосты из объекта.
	} // Конец проверки парсинга как объекта.

	// Пробуем распарсить как массив строк.
	var hostsArray []string                                   // Создаём переменную для массива хостов.
	if err := json.Unmarshal(body, &hostsArray); err == nil { // Если удалось распарсить как массив.
		return hostsArray, nil // Возвращаем массив хостов.
	} // Конец проверки парсинга как массива.

	return nil, fmt.Errorf("failed to parse hosts JSON: invalid format") // Если не удалось распарсить ни как объект, ни как массив, возвращаем ошибку.
} // Конец fetchHosts.

// checkHost проверяет актуальность кода на одном хосте.
// Отправляет GET запрос на https://<host>/api/v2/rooms/<code> с таймаутом 5 секунд.
// Возвращает указатель на RoomInfo, если хост вернул статус 200 и ответ успешно распарсен, иначе nil.
func checkHost(ctx context.Context, host, code string) *RoomInfo { // Функция проверки одного хоста.
	url := fmt.Sprintf("https://%s/api/v2/rooms/%s", host, code) // Формируем URL для запроса.

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Создаём HTTP GET запрос с контекстом.
	if err != nil {                                              // Если не удалось создать запрос.
		log.Printf("debug: failed to create request for %s: %v", host, err) // Логируем ошибку создания запроса.
		return nil                                                          // Возвращаем nil.
	} // Конец проверки создания запроса.

	// Устанавливаем заголовки для симуляции реального браузера.
	req.Header.Set("Accept", "*/*")                                                                                                                                     // Устанавливаем заголовок Accept.
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")                                                                                                        // Устанавливаем заголовок Accept-Encoding.
	req.Header.Set("Accept-Language", "ru,en;q=0.9")                                                                                                                    // Устанавливаем заголовок Accept-Language.
	req.Header.Set("Origin", "https://jackbox.fun")                                                                                                                     // Устанавливаем заголовок Origin.
	req.Header.Set("Priority", "u=1, i")                                                                                                                                // Устанавливаем заголовок Priority.
	req.Header.Set("Referer", "https://jackbox.fun/")                                                                                                                   // Устанавливаем заголовок Referer.
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="142", "YaBrowser";v="25.12", "Not_A Brand";v="99", "Yowser";v="2.5"`)                                                    // Устанавливаем заголовок Sec-Ch-Ua.
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")                                                                                                                            // Устанавливаем заголовок Sec-Ch-Ua-Mobile.
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)                                                                                                                   // Устанавливаем заголовок Sec-Ch-Ua-Platform.
	req.Header.Set("Sec-Fetch-Dest", "empty")                                                                                                                           // Устанавливаем заголовок Sec-Fetch-Dest.
	req.Header.Set("Sec-Fetch-Mode", "cors")                                                                                                                            // Устанавливаем заголовок Sec-Fetch-Mode.
	req.Header.Set("Sec-Fetch-Site", "cross-site")                                                                                                                      // Устанавливаем заголовок Sec-Fetch-Site.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 YaBrowser/25.12.0.0 Safari/537.36") // Устанавливаем заголовок User-Agent.

	client := &http.Client{Timeout: 5 * time.Second} // Создаём HTTP клиент с таймаутом 5 секунд.
	resp, err := client.Do(req)                      // Выполняем запрос.
	if err != nil {                                  // Если запрос не удался.
		// Проверяем, не была ли ошибка из-за отмены контекста (это нормально, если хост уже найден).
		if ctx.Err() == context.Canceled { // Если контекст был отменён.
			log.Printf("debug: request to %s was canceled (host already found)", host) // Логируем отмену запроса.
			return nil                                                                 // Возвращаем nil, так как хост уже найден другим запросом.
		} // Конец проверки отмены контекста.
		log.Printf("debug: request to %s failed: %v", host, err) // Логируем ошибку запроса.
		return nil                                               // Возвращаем nil.
	} // Конец проверки выполнения запроса.
	defer resp.Body.Close() // Закрываем тело ответа при выходе из функции.

	if resp.StatusCode != http.StatusOK { // Если статус не 200.
		log.Printf("debug: host %s returned status %d for code %s", host, resp.StatusCode, code) // Логируем статус ответа.
		return nil                                                                               // Возвращаем nil.
	} // Конец проверки статуса.

	// Читаем тело ответа для парсинга JSON.
	body, err := io.ReadAll(resp.Body) // Читаем всё тело ответа.
	if err != nil {                    // Если не удалось прочитать тело.
		log.Printf("debug: failed to read response body from %s: %v", host, err) // Логируем ошибку чтения тела.
		return nil                                                               // Возвращаем nil.
	} // Конец проверки чтения тела.

	// Парсим JSON ответ.
	var roomResp RoomResponse                               // Создаём переменную для ответа API.
	if err := json.Unmarshal(body, &roomResp); err != nil { // Пытаемся распарсить JSON.
		log.Printf("debug: failed to parse JSON response from %s: %v", host, err) // Логируем ошибку парсинга.
		return nil                                                                // Возвращаем nil.
	} // Конец проверки парсинга.

	if !roomResp.OK { // Если флаг ok не true.
		log.Printf("debug: host %s returned ok=false for code %s", host, code) // Логируем неуспешный ответ.
		return nil                                                             // Возвращаем nil.
	} // Конец проверки флага ok.

	log.Printf("debug: host %s returned 200 for code %s, room info: appTag=%s, host=%s", host, code, roomResp.Body.AppTag, roomResp.Body.Host) // Логируем успешный ответ с информацией о комнате.

	return &roomResp.Body // Возвращаем указатель на информацию о комнате.
} // Конец checkHost.

// findActiveHost проверяет все хосты параллельно и возвращает первый хост, который вернул 200.
// Как только один хост вернул 200, все остальные запросы отменяются.
// Возвращает найденный хост, информацию о комнате и nil, или пустую строку, nil и ошибку, если ни один хост не вернул 200.
func findActiveHost(hosts []string, code string) (string, *RoomInfo, error) { // Функция поиска актуального хоста.
	if len(hosts) == 0 { // Если список хостов пуст.
		return "", nil, fmt.Errorf("no hosts provided") // Возвращаем ошибку.
	} // Конец проверки списка хостов.

	ctx, cancel := context.WithCancel(context.Background()) // Создаём контекст с возможностью отмены.
	defer cancel()                                          // Отменяем контекст при выходе из функции.

	var wg sync.WaitGroup       // Создаём WaitGroup для синхронизации горутин.
	var mu sync.Mutex           // Создаём мьютекс для защиты общей переменной.
	var foundHost string        // Переменная для хранения найденного хоста.
	var foundRoomInfo *RoomInfo // Переменная для хранения информации о комнате.
	var found bool              // Флаг, указывающий, что хост найден.

	for _, host := range hosts { // Проходим по каждому хосту.
		wg.Add(1) // Увеличиваем счётчик WaitGroup.

		go func(h string) { // Запускаем горутину для проверки хоста.
			defer wg.Done() // Уменьшаем счётчик WaitGroup при выходе из горутины.

			mu.Lock()             // Блокируем мьютекс.
			alreadyFound := found // Проверяем, не найден ли уже хост.
			mu.Unlock()           // Разблокируем мьютекс.

			if alreadyFound { // Если хост уже найден.
				log.Printf("debug: skipping check for %s, host already found", h) // Логируем пропуск проверки.
				return                                                            // Выходим из горутины.
			} // Конец проверки флага.

			log.Printf("debug: checking host %s for code %s", h, code) // Логируем начало проверки хоста.

			roomInfo := checkHost(ctx, h, code) // Проверяем хост и получаем информацию о комнате.
			if roomInfo != nil {                // Если хост вернул успешный ответ.
				mu.Lock()   // Блокируем мьютекс.
				if !found { // Если хост ещё не найден.
					found = true                                                      // Устанавливаем флаг.
					foundHost = h                                                     // Сохраняем найденный хост.
					foundRoomInfo = roomInfo                                          // Сохраняем информацию о комнате.
					log.Printf("debug: host %s is active, canceling other checks", h) // Логируем найденный хост.
					cancel()                                                          // Отменяем все остальные запросы.
				} else { // Если хост уже найден другим запросом.
					log.Printf("debug: host %s returned 200, but another host was already found", h) // Логируем дубликат.
				} // Конец проверки флага.
				mu.Unlock() // Разблокируем мьютекс.
			} // Конец проверки хоста.
		}(host) // Передаём хост в горутину.
	} // Конец цикла по хостам.

	wg.Wait() // Ждём завершения всех горутин.

	mu.Lock()                       // Блокируем мьютекс.
	resultHost := foundHost         // Сохраняем результат хоста.
	resultRoomInfo := foundRoomInfo // Сохраняем результат информации о комнате.
	hasFound := found               // Сохраняем флаг.
	mu.Unlock()                     // Разблокируем мьютекс.

	if !hasFound { // Если хост не найден.
		return "", nil, fmt.Errorf("no active host found for code %s", code) // Возвращаем ошибку.
	} // Конец проверки результата.

	return resultHost, resultRoomInfo, nil // Возвращаем найденный хост, информацию о комнате и nil (ошибки нет, так как хост найден).
} // Конец findActiveHost.

// Ddos реализует команду ddos.
// Принимает args — аргументы командной строки после подкоманды "ddos".
// Формат: ddos <code> [nickname]
// code — обязательный, 4 буквы (любой регистр).
// nickname — необязательный, 6 символов.
func Ddos(args []string) { // Реализация команды ddos.
	if len(args) < 1 { // Если код не указан.
		log.Printf("error: code is required") // Пишем в лог.
		os.Exit(2)                            // Выходим.
	} // Конец проверки наличия кода.

	code := args[0] // Берём первый аргумент как код.

	// Валидация обязательного аргумента code.
	if len(code) != 4 { // Проверяем длину кода (должно быть 4 символа).
		log.Printf("error: code must be exactly 4 characters, got %d", len(code)) // Пишем в лог.
		os.Exit(2)                                                                // Выходим.
	} // Конец проверки длины кода.

	// Проверяем, что все символы в code — буквы (любой регистр).
	for _, r := range code { // Проходим по каждому символу кода.
		if !unicode.IsLetter(r) { // Если символ не является буквой.
			log.Printf("error: code must contain only letters, got invalid character: %c", r) // Пишем в лог.
			os.Exit(2)                                                                        // Выходим.
		} // Конец проверки символа.
	} // Конец цикла проверки символов.

	// Валидация необязательного аргумента nickname (если указан).
	var nickname string // Объявляем переменную для никнейма.
	if len(args) >= 2 { // Если указан второй аргумент.
		nickname = args[1]      // Берём второй аргумент как никнейм.
		if len(nickname) != 6 { // Проверяем длину никнейма (должно быть 6 символов).
			log.Printf("error: nickname must be exactly 6 characters, got %d", len(nickname)) // Пишем в лог.
			os.Exit(2)                                                                        // Выходим.
		} // Конец проверки длины никнейма.
	} // Конец проверки никнейма.

	// Загружаем список хостов из JSON файла.
	hostsURL := "https://gist.githubusercontent.com/Geardung/67d9695f4f09836364cbe724721d3046/raw/8a72727071a08134eb212a2f5f5df00792107c22/hosts.json" // URL для загрузки списка хостов.
	hosts, err := fetchHosts(hostsURL)                                                                                                                 // Загружаем хосты.
	if err != nil {                                                                                                                                    // Если не удалось загрузить хосты.
		log.Printf("error: failed to fetch hosts: %v", err) // Логируем ошибку.
		os.Exit(2)                                          // Выходим с кодом ошибки.
	} // Конец проверки загрузки хостов.

	if len(hosts) == 0 { // Если список хостов пуст.
		log.Printf("error: no hosts found") // Логируем ошибку.
		os.Exit(2)                          // Выходим с кодом ошибки.
	} // Конец проверки списка хостов.

	log.Printf("checking code %s on %d hosts", strings.ToUpper(code), len(hosts)) // Логируем начало проверки.

	// Проверяем все хосты параллельно и находим актуальный.
	activeHost, roomInfo, err := findActiveHost(hosts, strings.ToUpper(code)) // Ищем актуальный хост и получаем информацию о комнате.
	if err != nil {                                                           // Если не удалось найти актуальный хост.
		log.Printf("error: %v", err) // Логируем ошибку.
		os.Exit(2)                   // Выходим с кодом ошибки.
	} // Конец проверки результата поиска.

	log.Printf("active host found: %s", activeHost) // Логируем найденный хост.
	if roomInfo != nil {                            // Если информация о комнате получена.
		log.Printf("room info: appTag=%s, gameHost=%s, maxPlayers=%d", roomInfo.AppTag, roomInfo.Host, roomInfo.MaxPlayers) // Логируем информацию о комнате.
	} // Конец проверки информации о комнате.

	// Вызываем функцию заполнения (ddosing) с собранными данными.
	// Функция будет реализована позже пользователем.
	ddosing(activeHost, strings.ToUpper(code), nickname, roomInfo) // Вызываем функцию заполнения.

	// Выводим информацию пользователю.
	fmt.Printf("Code: %s\n", strings.ToUpper(code)) // Выводим код в верхнем регистре.
	fmt.Printf("Active host: %s\n", activeHost)     // Выводим найденный хост.
	if roomInfo != nil {                            // Если информация о комнате получена.
		fmt.Printf("Game host: %s\n", roomInfo.Host)         // Выводим хост игры.
		fmt.Printf("App tag: %s\n", roomInfo.AppTag)         // Выводим тег приложения.
		fmt.Printf("Max players: %d\n", roomInfo.MaxPlayers) // Выводим максимальное количество игроков.
	} // Конец проверки информации о комнате.
	if nickname != "" { // Если никнейм указан.
		fmt.Printf("Nickname: %s\n", nickname) // Выводим никнейм.
	} // Конец проверки никнейма.
} // Конец Ddos.

// generateRandomName генерирует случайное имя для бота, если nickname не указан.
// Возвращает строку вида "Bot" + 4 случайных символа.
func generateRandomName() string { // Функция генерации случайного имени.
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" // Набор символов для генерации имени.
	const nameLength = 4                                                             // Длина случайной части имени.

	b := make([]byte, nameLength) // Создаём слайс байт для случайных символов.
	rand.Read(b)                  // Заполняем слайс случайными байтами.

	result := make([]byte, nameLength) // Создаём слайс для результата.
	for i := range b {                 // Проходим по каждому байту.
		result[i] = charset[b[i]%byte(len(charset))] // Преобразуем байт в символ из charset.
	} // Конец цикла.

	return "Bot" + string(result) // Возвращаем "Bot" + случайные символы.
} // Конец generateRandomName.

// connectWebSocket создаёт одно WebSocket подключение к комнате игры.
// Принимает домен хоста, код комнаты, имя игрока и UUID пользователя.
// Возвращает ошибку, если подключение не удалось.
func connectWebSocket(hostDomain, code, playerName, userID string) error { // Функция создания WebSocket подключения.
	// Формируем URL с параметрами.
	wsURL := url.URL{ // Создаём структуру URL.
		Scheme:   "wss",                                                                                          // Используем протокол wss (WebSocket Secure).
		Host:     hostDomain,                                                                                     // Устанавливаем хост.
		Path:     fmt.Sprintf("/api/v2/rooms/%s/play", code),                                                     // Устанавливаем путь с кодом комнаты.
		RawQuery: fmt.Sprintf("role=player&name=%s&format=json&user-id=%s", url.QueryEscape(playerName), userID), // Устанавливаем параметры запроса.
	} // Конец создания URL.

	log.Printf("debug: connecting WebSocket to %s", wsURL.String()) // Логируем URL подключения.

	// Создаём заголовки для WebSocket handshake.
	// ВАЖНО: gorilla/websocket автоматически добавляет следующие заголовки:
	// - Connection: Upgrade
	// - Upgrade: websocket
	// - Sec-WebSocket-Version: 13
	// - Sec-WebSocket-Key (генерируется автоматически)
	// - Sec-WebSocket-Extensions (добавляется автоматически при EnableCompression: true)
	// - Sec-WebSocket-Protocol (устанавливается через Subprotocols в Dialer)
	// Поэтому мы НЕ устанавливаем эти заголовки вручную, чтобы избежать ошибки "duplicate header".
	header := make(http.Header)                                                                                                                                     // Создаём карту заголовков.
	header.Set("Host", hostDomain)                                                                                                                                  // Устанавливаем заголовок Host.
	header.Set("Pragma", "no-cache")                                                                                                                                // Устанавливаем заголовок Pragma.
	header.Set("Cache-Control", "no-cache")                                                                                                                         // Устанавливаем заголовок Cache-Control.
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 YaBrowser/25.12.0.0 Safari/537.36") // Устанавливаем заголовок User-Agent.
	header.Set("Origin", "https://jackbox.fun")                                                                                                                     // Устанавливаем заголовок Origin.
	header.Set("Accept-Encoding", "gzip, deflate, br, zstd")                                                                                                        // Устанавливаем заголовок Accept-Encoding.
	header.Set("Accept-Language", "ru,en;q=0.9")                                                                                                                    // Устанавливаем заголовок Accept-Language.

	// Создаём WebSocket dialer.
	dialer := websocket.Dialer{ // Создаём структуру Dialer для подключения.
		HandshakeTimeout:  10 * time.Second,     // Устанавливаем таймаут handshake в 10 секунд.
		EnableCompression: true,                 // Включаем сжатие (permessage-deflate).
		Subprotocols:      []string{"ecast-v0"}, // Устанавливаем подпротокол ecast-v0.
	} // Конец создания Dialer.

	// Подключаемся к WebSocket серверу.
	conn, resp, err := dialer.Dial(wsURL.String(), header) // Выполняем подключение с заголовками.
	if err != nil {                                        // Если подключение не удалось.
		if resp != nil { // Если ответ получен.
			log.Printf("debug: WebSocket connection failed for %s: %v (status: %d)", playerName, err, resp.StatusCode) // Логируем ошибку с статусом.
		} else { // Если ответа нет.
			log.Printf("debug: WebSocket connection failed for %s: %v", playerName, err) // Логируем ошибку без статуса.
		} // Конец проверки ответа.
		return fmt.Errorf("failed to connect WebSocket: %w", err) // Возвращаем ошибку.
	} // Конец проверки подключения.

	log.Printf("debug: WebSocket connected successfully for %s (user-id: %s)", playerName, userID) // Логируем успешное подключение.

	// Подключение установлено, теперь нужно поддерживать его активным.
	// Читаем сообщения от сервера в бесконечном цикле, чтобы соединение оставалось открытым.
	// Устанавливаем таймаут чтения, чтобы периодически проверять, не закрыто ли соединение.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // Устанавливаем таймаут чтения в 60 секунд.

	// Запускаем горутину для чтения сообщений (чтобы не блокировать возврат функции).
	go func() { // Запускаем горутину для чтения сообщений.
		defer conn.Close() // Закрываем соединение при выходе из горутины.

		for { // Бесконечный цикл чтения сообщений.
			// Читаем сообщение от сервера (тип сообщения не важен, главное - поддерживать соединение).
			_, message, err := conn.ReadMessage() // Читаем сообщение.
			if err != nil {                       // Если произошла ошибка чтения.
				// Проверяем, не была ли это ошибка таймаута (это нормально, просто обновляем таймаут).
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) { // Если это неожиданное закрытие.
					log.Printf("debug: WebSocket read error for %s: %v", playerName, err) // Логируем ошибку.
					return                                                                // Выходим из цикла.
				} // Конец проверки ошибки.
				// Если это таймаут, просто обновляем его и продолжаем.
				conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // Обновляем таймаут чтения.
				continue                                               // Продолжаем цикл.
			} // Конец проверки ошибки.

			// Сообщение получено (можно добавить обработку позже).
			_ = message // Игнорируем содержимое сообщения пока.

			// Обновляем таймаут для следующего чтения.
			conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // Обновляем таймаут чтения.
		} // Конец бесконечного цикла.
	}() // Запускаем горутину.

	// Функция возвращается, но соединение остаётся открытым в горутине.

	return nil // Возвращаем nil (ошибки нет).
} // Конец connectWebSocket.

// ddosing выполняет основную логику заполнения комнаты ботами.
// Принимает домен хоста, код комнаты, опциональный никнейм и информацию о комнате.
// Создаёт 10 параллельных WebSocket подключений для заполнения слотов.
func ddosing(hostDomain, code, nickname string, roomInfo *RoomInfo) { // Функция заполнения комнаты.
	log.Printf("ddosing: hostDomain=%s, code=%s, nickname=%s", hostDomain, code, nickname) // Логируем параметры функции заполнения.

	const numConnections = 10 // Количество параллельных подключений.

	var wg sync.WaitGroup // Создаём WaitGroup для синхронизации горутин.
	var mu sync.Mutex     // Создаём мьютекс для защиты счётчика.
	var successCount int  // Счётчик успешных подключений.
	var failCount int     // Счётчик неуспешных подключений.

	// Определяем имя для использования (если nickname пустой, генерируем случайное для каждого подключения).
	useNickname := nickname != "" // Проверяем, указан ли nickname.

	log.Printf("starting %d WebSocket connections...", numConnections) // Логируем начало подключений.

	for i := 0; i < numConnections; i++ { // Проходим по количеству подключений.
		wg.Add(1) // Увеличиваем счётчик WaitGroup.

		go func(connNum int) { // Запускаем горутину для каждого подключения.
			defer wg.Done() // Уменьшаем счётчик WaitGroup при выходе из горутины.

			// Генерируем уникальный UUID для каждого подключения.
			userID := uuid.New().String() // Генерируем новый UUID.

			// Определяем имя игрока.
			var playerName string // Объявляем переменную для имени игрока.
			if useNickname {      // Если nickname указан.
				playerName = nickname // Используем указанный nickname.
			} else { // Если nickname не указан.
				playerName = generateRandomName() // Генерируем случайное имя.
			} // Конец проверки nickname.

			// Добавляем номер подключения к имени для уникальности (если nickname не указан).
			if !useNickname { // Если nickname не указан.
				playerName = fmt.Sprintf("%s%d", playerName, connNum) // Добавляем номер к имени.
			} else { // Если nickname указан.
				playerName = fmt.Sprintf("%s%d", playerName, connNum) // Добавляем номер к имени для уникальности.
			} // Конец проверки nickname.

			// Создаём WebSocket подключение.
			err := connectWebSocket(hostDomain, code, playerName, userID) // Выполняем подключение.
			if err != nil {                                               // Если подключение не удалось.
				mu.Lock()                                            // Блокируем мьютекс.
				failCount++                                          // Увеличиваем счётчик неудач.
				mu.Unlock()                                          // Разблокируем мьютекс.
				log.Printf("connection %d failed: %v", connNum, err) // Логируем ошибку подключения.
				return                                               // Выходим из горутины.
			} // Конец проверки ошибки.

			// Подключение успешно установлено.
			mu.Lock()                                                                              // Блокируем мьютекс.
			successCount++                                                                         // Увеличиваем счётчик успехов.
			mu.Unlock()                                                                            // Разблокируем мьютекс.
			log.Printf("connection %d established: %s (user-id: %s)", connNum, playerName, userID) // Логируем успешное подключение.

			// Держим соединение открытым (можно добавить логику обработки сообщений позже).
			// Пока просто ждём, чтобы соединение оставалось активным.
			// В реальном сценарии здесь может быть цикл чтения сообщений от сервера.
		}(i) // Передаём номер подключения в горутину.
	} // Конец цикла подключений.

	wg.Wait() // Ждём завершения всех горутин.

	// Выводим итоговую статистику.
	log.Printf("ddosing completed: %d successful, %d failed", successCount, failCount) // Логируем итоговую статистику.
	fmt.Printf("Connections: %d successful, %d failed\n", successCount, failCount)     // Выводим статистику пользователю.
} // Конец ddosing.
