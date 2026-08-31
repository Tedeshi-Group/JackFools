package commands // Пакет commands содержит реализацию CLI команд.

import ( // Начинаем блок импортов.
	"context"       // Отменяем запросы и горутины при завершении.
	"crypto/rand"   // Генерируем криптографически стойкие случайные числа для имён зрителей.
	"encoding/json" // Парсим JSON сообщения от WebSocket.
	"fmt"           // Печатаем сообщения пользователю.
	"log"           // Логируем события сервера/ошибки.
	"net/http"      // Используем http.Header для WebSocket заголовков.
	"net/url"       // Работаем с URL параметрами.
	"os"            // Читаем аргументы и завершаем процесс с кодом.
	"os/signal"     // Обрабатываем сигналы для graceful shutdown.
	"strconv"       // Преобразуем строки в числа.
	"strings"       // Работаем со строками для валидации.
	"sync"          // Синхронизируем горутины.
	"syscall"       // Используем syscall.SIGINT, syscall.SIGTERM для обработки сигналов.
	"time"          // Устанавливаем таймауты для запросов.
	"unicode"       // Проверяем, является ли символ буквой.

	"github.com/google/uuid"       // Генерируем UUID для user-id.
	"github.com/gorilla/websocket" // Работаем с WebSocket подключениями.

	_ "jackfools/client/internal/games/generic" // Импортируем для init() регистрации generic handler.
	_ "jackfools/client/internal/games/fakinit" // Импортируем для init() регистрации Fakin' It handler.
	_ "jackfools/client/internal/games/tsjp"    // Импортируем для init() регистрации TSJP handler.
	"jackfools/client/internal/server"          // Общие типы вопросов (QuestionBank).
) // Закрываем блок импортов.

// BotnetManager управляет всеми подключениями ботнета.
// Координирует работу координатора и клиентов.
type BotnetManager struct { // Структура менеджера ботнета.
	mu               sync.RWMutex            // Мьютекс для защиты общих данных.
	coordinator      *websocket.Conn         // WebSocket соединение координатора.
	clients          map[int]*websocket.Conn // Карта клиентов по их ID.
	ctx              context.Context         // Контекст для отмены операций.
	cancel           context.CancelFunc      // Функция отмены контекста.
	answerDBTJSP     *AnswerDatabase         // База правильных ответов для triviadeath2-tjsp (обычные вопросы).
	finalRoundDBTJSP *AnswerDatabase         // База правильных ответов для triviadeath2-tjsp (финальный раунд).
	commandChan      chan ClientCommand      // Канал для отправки команд клиентам.
	gameTag          string                  // Кешированный тег игры (извлекается из первого сообщения).
	everydayTimes    int                     // Счётчик times для игры Everyday (начинается с 50, увеличивается).
	currentQuestion  *CurrentQuestion        // Текущий вопрос для обучения.
	role             string                  // Роль ботов: "player" или "audience".
} // Конец BotnetManager.

// CurrentQuestion хранит контекст текущего вопроса для обучения.
type CurrentQuestion struct { // Контекст текущего вопроса.
	Prompt  string   // Текст вопроса.
	Choices []string // Варианты ответов.
} // Конец CurrentQuestion.

// ClientCommand представляет команду, которую координатор отправляет клиентам.
type ClientCommand struct { // Структура команды клиенту.
	Type    string                 // Тип команды (например, "answer").
	EventID string                 // ID события, на которое нужно ответить.
	Answer  string                 // Ответ для отправки.
	Payload map[string]interface{} // Дополнительные данные команды.
} // Конец ClientCommand.

// Botnet реализует команду botnet.
// Принимает args — аргументы командной строки после подкоманды "botnet".
// Формат: botnet <code> [num_clients] [role]
// code — обязательный, 4 буквы (любой регистр).
// num_clients — необязательный, количество клиентов (по умолчанию 10).
// role — необязательный, "player" или "audience" (по умолчанию "audience").
func Botnet(args []string) { // Реализация команды botnet.
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

	// Валидация необязательного аргумента num_clients.
	numClients := 10    // Значение по умолчанию.
	if len(args) >= 2 { // Если указан второй аргумент.
		var err error                           // Переменная для ошибки.
		numClients, err = strconv.Atoi(args[1]) // Преобразуем строку в число.
		if err != nil {                         // Если преобразование не удалось.
			log.Printf("error: num_clients must be a number, got: %s", args[1]) // Пишем в лог.
			os.Exit(2)                                                          // Выходим.
		} // Конец проверки преобразования.
		if numClients < 1 { // Если количество клиентов меньше 1.
			log.Printf("error: num_clients must be at least 1, got: %d", numClients) // Пишем в лог.
			os.Exit(2)                                                               // Выходим.
		} // Конец проверки минимального значения.
		if numClients > 100 { // Если количество клиентов больше 100.
			log.Printf("error: num_clients must be at most 100, got: %d", numClients) // Пишем в лог.
			os.Exit(2)                                                                // Выходим.
		} // Конец проверки максимального значения.
	} // Конец проверки num_clients.

	// Валидация необязательного аргумента role.
	role := "audience" // Значение по умолчанию — зрители.
	if len(args) >= 3 { // Если указан третий аргумент.
		role = strings.ToLower(args[2]) // Преобразуем к нижнему регистру.
		if role != "player" && role != "audience" { // Если роль неизвестна.
			log.Printf("error: role must be 'player' or 'audience', got: %s", args[2]) // Пишем в лог.
			os.Exit(2)                                                                  // Выходим.
		} // Конец проверки роли.
	} // Конец проверки role.

	// Загружаем список хостов из JSON файла.
	hostsURL := defaultHostsURL // URL для загрузки списка хостов.
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
	if roomInfo == nil {                            // Если информация о комнате не получена.
		log.Printf("error: room info is required") // Логируем ошибку.
		os.Exit(2)                                 // Выходим с кодом ошибки.
	} // Конец проверки информации о комнате.

	if roomInfo.AudienceHost == "" { // Если хост для аудитории не указан.
		log.Printf("error: audience host is not available for this room") // Логируем ошибку.
		os.Exit(2)                                                        // Выходим с кодом ошибки.
	} // Конец проверки хоста аудитории.

	log.Printf("room info: appTag=%s, audienceHost=%s, maxPlayers=%d", roomInfo.AppTag, roomInfo.AudienceHost, roomInfo.MaxPlayers) // Логируем информацию о комнате.

	// Создаём контекст с возможностью отмены.
	ctx, cancel := context.WithCancel(context.Background()) // Создаём контекст с отменой.
	defer cancel()                                          // Отменяем контекст при выходе из функции.

	// Обрабатываем сигналы для graceful shutdown.
	sigChan := make(chan os.Signal, 1)                      // Создаём канал для сигналов.
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM) // Подписываемся на SIGINT и SIGTERM.

	// Запускаем горутину для обработки сигналов.
	go func() { // Запускаем горутину для обработки сигналов.
		<-sigChan                                                      // Ждём сигнал.
		log.Printf("received shutdown signal, closing connections...") // Логируем получение сигнала.
		cancel()                                                       // Отменяем контекст, что приведёт к закрытию всех подключений.
	}() // Запускаем горутину.

	// Загружаем базу ответов для triviadeath2-tjsp (обычные вопросы).
	answersPathTJSP := "answers/triviadeath2-tjsp-questions.json" // Локальный путь к базе ответов для TJSP.
	answerDBTJSP, err := loadAnswerDatabase(answersPathTJSP)      // Загружаем базу ответов для TJSP.
	if err != nil {                                                                                                                                                                // Если не удалось загрузить базу ответов.
		log.Printf("warning: failed to load TJSP answer database: %v, continuing without auto-answers", err) // Логируем предупреждение.
		answerDBTJSP = &AnswerDatabase{}                                                                     // Создаём пустую базу ответов.
	} // Конец проверки загрузки базы ответов.

	// Загружаем базу ответов для triviadeath2-tjsp (финальный раунд).
	finalRoundPathTJSP := "answers/triviadeath2-tjsp-final.json"   // Локальный путь к базе финального раунда для TJSP.
	finalRoundDBTJSP, err := loadFinalRoundDatabase(finalRoundPathTJSP) // Загружаем базу ответов финального раунда для TJSP.
	if err != nil {                                                                                                                                                               // Если не удалось загрузить базу ответов финального раунда.
		log.Printf("warning: failed to load TJSP final round database: %v, continuing without final round auto-answers", err) // Логируем предупреждение.
		finalRoundDBTJSP = &AnswerDatabase{                                                                                   // Создаём пустую базу ответов.
			FinalRoundQuestions: make(map[string][]string), // Инициализируем карту вопросов финального раунда.
		} // Конец создания базы.
	} // Конец проверки загрузки базы ответов финального раунда.

	// Создаём менеджер ботнета.
	manager := &BotnetManager{ // Создаём менеджер.
		clients:          make(map[int]*websocket.Conn), // Инициализируем карту клиентов.
		ctx:              ctx,                           // Устанавливаем контекст.
		cancel:           cancel,                        // Устанавливаем функцию отмены.
		answerDBTJSP:     answerDBTJSP,                  // Устанавливаем базу ответов для triviadeath2-tjsp (обычные вопросы).
		finalRoundDBTJSP: finalRoundDBTJSP,              // Устанавливаем базу ответов для triviadeath2-tjsp (финальный раунд).
		commandChan:      make(chan ClientCommand, 100), // Создаём канал для команд (буфер 100).
		// TODO: The reason for starting at 50 is unknown. The `times` parameter in
		// `audience/g-counter/increment` controls how much each bot's vote increments
		// the counter. Starting at 50 gives each bot's first vote more weight, but
		// the exact reason for choosing 50 (vs 1 or another value) is unclear — it
		// may have been tuned empirically to achieve a desired counter offset, or it
		// could be a leftover from experimentation. If the game works correctly with
		// a different starting value, this should be revisited.
		everydayTimes:    50,                            // Инициализируем счётчик times для Everyday (начинаем с 50, см. TODO выше).
		gameTag:          roomInfo.AppTag,               // Устанавливаем тег игры из информации о комнате (если доступен).
		role:             role,                          // Устанавливаем роль ботов.
	} // Конец создания менеджера.

	// Подключаем координатора.
	coordinatorID := uuid.New().String()                                                              // Генерируем UUID для координатора.
	coordConn, err := connectAsRole(ctx, roomInfo.AudienceHost, strings.ToUpper(code), coordinatorID, role) // Подключаемся в указанной роли.
	if err != nil {                                                                                   // Если подключение не удалось.
		log.Printf("error: failed to connect coordinator: %v", err) // Логируем ошибку.
		os.Exit(2)                                                  // Выходим с кодом ошибки.
	} // Конец проверки подключения координатора.

	manager.coordinator = coordConn                  // Сохраняем соединение координатора.
	log.Printf("coordinator connected successfully") // Логируем успешное подключение координатора.

	// Запускаем цикл координатора в отдельной горутине.
	go runCoordinator(coordConn, strings.ToUpper(code), manager) // Запускаем координатор.

	// Подключаем клиентов.
	var wg sync.WaitGroup             // Создаём WaitGroup для синхронизации горутин.
	for i := 0; i < numClients; i++ { // Проходим по количеству клиентов.
		wg.Add(1) // Увеличиваем счётчик WaitGroup.

		go func(clientID int) { // Запускаем горутину для каждого клиента.
			defer wg.Done() // Уменьшаем счётчик WaitGroup при выходе из горутины.

			clientUUID := uuid.New().String()                                                             // Генерируем UUID для клиента.
			clientConn, err := connectAsRole(ctx, roomInfo.AudienceHost, strings.ToUpper(code), clientUUID, role) // Подключаемся в указанной роли.
			if err != nil {                                                                                     // Если подключение не удалось.
				log.Printf("error: failed to connect client %d: %v", clientID, err) // Логируем ошибку.
				return                                                              // Выходим из горутины.
			} // Конец проверки подключения.

			manager.mu.Lock()                      // Блокируем мьютекс.
			manager.clients[clientID] = clientConn // Сохраняем соединение клиента.
			manager.mu.Unlock()                    // Разблокируем мьютекс.

			log.Printf("client %d connected successfully", clientID) // Логируем успешное подключение клиента.

			// Запускаем цикл клиента.
			runClient(clientConn, clientID, manager) // Запускаем клиента.
		}(i) // Передаём ID клиента в горутину.
	} // Конец цикла подключения клиентов.

	// Выводим информацию пользователю.
	fmt.Printf("Botnet started:\n")                            // Выводим заголовок.
	fmt.Printf("  Code: %s\n", strings.ToUpper(code))          // Выводим код.
	fmt.Printf("  Active host: %s\n", activeHost)              // Выводим найденный хост.
	fmt.Printf("  Audience host: %s\n", roomInfo.AudienceHost) // Выводим хост аудитории.
	fmt.Printf("  App tag: %s\n", roomInfo.AppTag)             // Выводим тег приложения.
	fmt.Printf("  Role: %s\n", role)                           // Выводим роль ботов.
	fmt.Printf("  Clients: %d\n", numClients)                  // Выводим количество клиентов.
	fmt.Printf("  Press Ctrl+C to stop\n")                     // Выводим подсказку.

	// Ждём завершения всех горутин или отмены контекста.
	wg.Wait() // Ждём завершения всех горутин клиентов.

	log.Printf("botnet stopped") // Логируем остановку ботнета.
} // Конец Botnet.

// connectAsRole подключается к комнате игры в указанной роли (player или audience).
func connectAsRole(ctx context.Context, host, code, userID, role string) (*websocket.Conn, error) {
	if role == "player" {
		return connectAsPlayer(ctx, host, code, userID)
	}
	return connectAsAudience(ctx, host, code, userID)
}

// connectAsPlayer создаёт WebSocket подключение к комнате игры как игрок.
func connectAsPlayer(ctx context.Context, host, code, userID string) (*websocket.Conn, error) {
	playerName := generateRandomAudienceName() // Генерируем имя игрока.

	wsURL := url.URL{
		Scheme:   "wss",
		Host:     host,
		Path:     fmt.Sprintf("/api/v2/audience/%s/play", code),
		RawQuery: fmt.Sprintf("role=player&name=%s&format=json&user-id=%s", url.QueryEscape(playerName), userID),
	}

	log.Printf("debug: connecting WebSocket as player to %s", wsURL.String())

	header := make(http.Header)
	header.Set("Host", host)
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 YaBrowser/25.12.0.0 Safari/537.36")
	header.Set("Origin", "https://jackbox.fun")
	header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	header.Set("Accept-Language", "ru,en;q=0.9")

	dialer := websocket.Dialer{
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: true,
		Subprotocols:      []string{"ecast-v0"},
	}

	conn, resp, err := dialer.Dial(wsURL.String(), header)
	if err != nil {
		if resp != nil {
			log.Printf("debug: player connection failed: %v (status: %d)", err, resp.StatusCode)
		} else {
			log.Printf("debug: player connection failed: %v", err)
		}
		return nil, fmt.Errorf("failed to connect as player: %w", err)
	}

	select {
	case <-ctx.Done():
		conn.Close()
		return nil, fmt.Errorf("context canceled before connection established")
	default:
	}

	log.Printf("debug: WebSocket connected successfully as player (user-id: %s)", userID)
	return conn, nil
}

// connectAsAudience создаёт WebSocket подключение к комнате игры как зритель.
// Принимает контекст, домен хоста аудитории, код комнаты и UUID пользователя.
// Возвращает WebSocket соединение или ошибку.
func connectAsAudience(ctx context.Context, audienceHost, code, userID string) (*websocket.Conn, error) { // Функция подключения как зритель.
	// Генерируем случайное имя для зрителя (4 символа).
	audienceName := generateRandomAudienceName() // Генерируем случайное имя.

	// Формируем URL с параметрами для зрителя.
	// Правильный путь для зрителей: /api/v2/audience/{code}/play
	wsURL := url.URL{ // Создаём структуру URL.
		Scheme:   "wss",                                                                                              // Используем протокол wss (WebSocket Secure).
		Host:     audienceHost,                                                                                       // Устанавливаем хост аудитории.
		Path:     fmt.Sprintf("/api/v2/audience/%s/play", code),                                                      // Устанавливаем правильный путь для зрителей.
		RawQuery: fmt.Sprintf("role=audience&name=%s&format=json&user-id=%s", url.QueryEscape(audienceName), userID), // Устанавливаем параметры запроса (role=audience, name обязателен, userID не требует экранирования, так как это UUID).
	} // Конец создания URL.

	log.Printf("debug: connecting WebSocket as audience to %s", wsURL.String()) // Логируем URL подключения.

	// Создаём заголовки для WebSocket handshake.
	header := make(http.Header)                                                                                                                                     // Создаём карту заголовков.
	header.Set("Host", audienceHost)                                                                                                                                // Устанавливаем заголовок Host.
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
	// Используем Dial вместо DialContext, так как контекст нужен только для отмены,
	// а мы проверяем его отдельно перед подключением.
	conn, resp, err := dialer.Dial(wsURL.String(), header) // Выполняем подключение с заголовками.
	if err != nil {                                        // Если подключение не удалось.
		if resp != nil { // Если ответ получен.
			log.Printf("debug: WebSocket connection failed: %v (status: %d)", err, resp.StatusCode) // Логируем ошибку с статусом.
		} else { // Если ответа нет.
			log.Printf("debug: WebSocket connection failed: %v", err) // Логируем ошибку без статуса.
		} // Конец проверки ответа.
		return nil, fmt.Errorf("failed to connect WebSocket: %w", err) // Возвращаем ошибку.
	} // Конец проверки подключения.

	// Проверяем, не отменён ли контекст после подключения.
	select { // Выбираем между контекстом.
	case <-ctx.Done(): // Если контекст отменён.
		conn.Close()                                                             // Закрываем соединение.
		return nil, fmt.Errorf("context canceled before connection established") // Возвращаем ошибку.
	default: // Если контекст не отменён, продолжаем.
	} // Конец select.

	log.Printf("debug: WebSocket connected successfully as audience (user-id: %s)", userID) // Логируем успешное подключение.

	return conn, nil // Возвращаем соединение и nil (ошибки нет).
} // Конец connectAsAudience.

// runCoordinator запускает главный цикл координатора.
// Читает события от сервера и управляет клиентами.
// Принимает WebSocket соединение, код комнаты и менеджер ботнета.
func runCoordinator(conn *websocket.Conn, code string, manager *BotnetManager) { // Функция запуска координатора.
	defer conn.Close() // Закрываем соединение при выходе из функции.

	for { // Бесконечный цикл чтения событий.
		// Проверяем, не отменён ли контекст.
		select { // Выбираем между контекстом и чтением сообщения.
		case <-manager.ctx.Done(): // Если контекст отменён.
			log.Printf("coordinator: context canceled, shutting down") // Логируем отмену контекста.
			return                                                     // Выходим из цикла.
		default: // Если контекст не отменён, продолжаем.
		} // Конец select.

		// Отключаем таймаут чтения (устанавливаем в нулевое время для отключения).
		conn.SetReadDeadline(time.Time{}) // Отключаем таймаут чтения.

		// Читаем сообщение от сервера.
		_, message, err := conn.ReadMessage() // Читаем сообщение.
		if err != nil {                       // Если произошла ошибка чтения.
			// Проверяем, не была ли это ошибка отмены контекста.
			if manager.ctx.Err() != nil { // Если контекст отменён.
				log.Printf("coordinator: context canceled") // Логируем отмену контекста.
				return                                      // Выходим из цикла.
			} // Конец проверки контекста.
			// Проверяем, не закрыто ли соединение.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) { // Если это неожиданное закрытие.
				log.Printf("coordinator: WebSocket read error: %v", err) // Логируем ошибку.
				return                                                   // Выходим из цикла.
			} // Конец проверки ошибки.
			// Если это другая ошибка, логируем и выходим.
			log.Printf("coordinator: WebSocket read error: %v", err) // Логируем ошибку.
			return                                                   // Выходим из цикла.
		} // Конец проверки ошибки.

		log.Printf("[JF-COORD] raw message (len=%d): %s", len(message), string(message[:min(300, len(message))])) // Логируем сырое сообщение.

		// Парсим и обрабатываем сообщение.
		msg, err := parseWebSocketMessage(message) // Парсим сообщение.
		if err != nil {                            // Если парсинг не удался.
			log.Printf("coordinator: failed to parse message: %v", err) // Логируем ошибку парсинга.
			continue                                                    // Продолжаем цикл.
		} // Конец проверки парсинга.

		// Преобразуем WebSocket сообщение в GameEvent.
		event, err := parseGameEvent(msg) // Преобразуем сообщение в событие.
		if err != nil {                   // Если преобразование не удалось.
			log.Printf("coordinator: failed to parse game event: %v", err) // Логируем ошибку преобразования.
			continue                                                       // Продолжаем цикл.
		} // Конец проверки преобразования.

		// Если gameTag извлечён из события и ещё не кеширован, сохраняем его.
		if event.GameTag != "" && manager.gameTag == "" { // Если тег игры найден и ещё не кеширован.
			manager.mu.Lock()                                             // Блокируем мьютекс.
			manager.gameTag = event.GameTag                               // Сохраняем тег игры.
			manager.mu.Unlock()                                           // Разблокируем мьютекс.
			log.Printf("coordinator: cached game tag: %s", event.GameTag) // Логируем кеширование тега игры.
		} // Конец проверки кеширования.

		// Если gameTag не найден в событии, но есть в кеше, используем кешированный.
		if event.GameTag == "" && manager.gameTag != "" { // Если тег игры не найден, но есть в кеше.
			event.GameTag = manager.gameTag // Используем кешированный тег.
			// Переопределяем requiresAnswer после установки gameTag из кеша.
			event.RequiresAnswer = shouldRequireAnswer(event) // Переопределяем необходимость ответа.
		} // Конец проверки использования кеша.

		// Обрабатываем событие.
		if err := handleEvent(event, manager); err != nil { // Обрабатываем событие.
			log.Printf("coordinator: failed to handle event: %v", err) // Логируем ошибку обработки.
			// Продолжаем работу, даже если обработка не удалась.
		} // Конец проверки обработки.

		// Обучение: проверяем, не является ли сообщение ответом на вопрос.
		if event.Type == "object" { // Если opcode = "object".
			tryLearnFromMessage(message, manager) // Пытаемся извлечь правильный ответ.
		} // Конец проверки обучения.
	} // Конец бесконечного цикла.
} // Конец runCoordinator.

// runClient запускает цикл клиента.
// Клиент слушает команды от координатора и отправляет ответы.
// Принимает WebSocket соединение, ID клиента и менеджер ботнета.
func runClient(conn *websocket.Conn, clientID int, manager *BotnetManager) { // Функция запуска клиента.
	defer conn.Close() // Закрываем соединение при выходе из функции.

	// Запускаем горутину для чтения сообщений от сервера (чтобы поддерживать соединение).
	go func() { // Запускаем горутину для чтения сообщений.
		for { // Бесконечный цикл чтения сообщений.
			// Проверяем, не отменён ли контекст.
			select { // Выбираем между контекстом и чтением сообщения.
			case <-manager.ctx.Done(): // Если контекст отменён.
				return // Выходим из цикла.
			default: // Если контекст не отменён, продолжаем.
			} // Конец select.

			// Отключаем таймаут чтения (устанавливаем в нулевое время для отключения).
			conn.SetReadDeadline(time.Time{}) // Отключаем таймаут чтения.

			// Читаем сообщение от сервера.
			_, message, err := conn.ReadMessage() // Читаем сообщение.
			if err != nil {                       // Если произошла ошибка чтения.
				if manager.ctx.Err() != nil { // Если контекст отменён.
					return // Выходим из цикла.
				} // Конец проверки контекста.
				// Проверяем, не закрыто ли соединение.
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) { // Если это неожиданное закрытие.
					log.Printf("client %d: WebSocket read error: %v", clientID, err) // Логируем ошибку.
					return                                                           // Выходим из цикла.
				} // Конец проверки ошибки.
				// Если это другая ошибка, логируем и выходим.
				log.Printf("client %d: WebSocket read error: %v", clientID, err) // Логируем ошибку.
				return                                                           // Выходим из цикла.
			} // Конец проверки ошибки.

			// Игнорируем сообщения от сервера (координатор обрабатывает их).
			_ = message // Игнорируем содержимое сообщения.
		} // Конец бесконечного цикла.
	}() // Запускаем горутину.

	// Слушаем команды от координатора.
	for { // Бесконечный цикл ожидания команд.
		// Проверяем, не отменён ли контекст.
		select { // Выбираем между контекстом и командой.
		case <-manager.ctx.Done(): // Если контекст отменён.
			log.Printf("client %d: context canceled, shutting down", clientID) // Логируем отмену контекста.
			return                                                             // Выходим из цикла.
		case cmd := <-manager.commandChan: // Если получена команда от координатора.
			// Отправляем ответ на команду.
			if err := sendClientResponse(conn, cmd); err != nil { // Отправляем ответ.
				log.Printf("client %d: failed to send response: %v", clientID, err) // Логируем ошибку отправки.
			} else { // Если отправка успешна.
				log.Printf("client %d: sent response for event %s", clientID, cmd.EventID) // Логируем успешную отправку.
			} // Конец проверки отправки.
		} // Конец select.
	} // Конец бесконечного цикла.
} // Конец runClient.

// sendClientResponse отправляет ответ от клиента на сервер.
// Принимает WebSocket соединение и команду от координатора.
// Возвращает ошибку, если отправка не удалась.
func sendClientResponse(conn *websocket.Conn, cmd ClientCommand) error { // Функция отправки ответа клиента.
	var ( // Объявляем переменные для сообщения и ошибки.
		message map[string]interface{} // Сообщение для отправки.
		err     error                 // Ошибка построения сообщения.
	) // Конец объявления переменных.

	gameTag, _ := cmd.Payload["gameTag"].(string) // Извлекаем gameTag из payload.

	switch { // Диспетчер по типу игры.
	case gameTag == "everyday": // Если это Everyday.
		message, err = buildEverydayMessage(cmd) // Строим сообщение Everyday.
	case gameTag == "triviadeath2-tjsp" || strings.Contains(gameTag, "triviadeath2"): // Если это TriviaDeath2.
		playerMode, _ := cmd.Payload["playerMode"].(bool)   // Флаг player-режима.
		isFinalRound, _ := cmd.Payload["isFinalRound"].(bool) // Флаг финального раунда.
		switch { // Диспетчер по подтипу TriviaDeath2.
		case playerMode: // Player-режим (object/update).
			message, err = buildTD2PlayerMessage(cmd) // Строим сообщение player.
		case isFinalRound: // Финальный раунд (старый/новый формат).
			message, err = buildTD2FinalRoundMessage(cmd) // Строим сообщение финального раунда.
		default: // Обычный раунд audience (старый/новый формат).
			message, err = buildTD2AudienceMessage(cmd) // Строим сообщение audience.
		} // Конец диспетчера TriviaDeath2.
	default: // Все остальные игры (Poll Position, generic и т.д.).
		message, err = buildDefaultMessage(cmd) // Строим сообщение по умолчанию.
	} // Конец диспетчера.

	if err != nil { // Если построение сообщения вернуло ошибку.
		return err // Возвращаем ошибку.
	} // Конец проверки ошибки.

	// Кодируем сообщение в JSON.
	data, err := json.Marshal(message) // Кодируем сообщение в JSON.
	if err != nil {                    // Если кодирование не удалось.
		return fmt.Errorf("failed to marshal response: %w", err) // Возвращаем ошибку.
	} // Конец проверки кодирования.

	// Отправляем сообщение через WebSocket.
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil { // Отправляем сообщение.
		return fmt.Errorf("failed to write message: %w", err) // Возвращаем ошибку.
	} // Конец проверки отправки.

	return nil // Возвращаем nil (ошибки нет).
} // Конец sendClientResponse.

// buildEverydayMessage строит сообщение для игры Everyday.
// Извлекает opcode, key и times из payload и формирует JSON.
func buildEverydayMessage(cmd ClientCommand) (map[string]interface{}, error) { // Построение сообщения Everyday.
	opcode, _ := cmd.Payload["opcode"].(string) // Код операции для Everyday.
	key, _ := cmd.Payload["key"].(string)       // Key из события.
	times, _ := cmd.Payload["times"].(int)      // Times (начинается с 50, увеличивается).

	message := map[string]interface{}{ // Создаём карту для сообщения.
		"seq":    1,      // Порядковый номер сообщения (начинаем с 1).
		"opcode": opcode, // Код операции для Everyday.
		"params": map[string]interface{}{ // Параметры сообщения.
			"key":   key,   // Key из события.
			"times": times, // Times (начинается с 50, увеличивается).
		}, // Конец параметров.
	} // Конец создания сообщения.

	return message, nil // Возвращаем сообщение без ошибки.
} // Конец buildEverydayMessage.

// buildTD2PlayerMessage строит сообщение object/update для player-режима TriviaDeath2.
// Обрабатывает действия: submit, select/unselect, select_grid.
func buildTD2PlayerMessage(cmd ClientCommand) (map[string]interface{}, error) { // Построение сообщения player.
	responseKey, _ := cmd.Payload["responseKey"].(string) // Ключ ответа (choose:N, grid:N, finalround:N).
	action, _ := cmd.Payload["action"].(string)          // Действие игрока.

	val := map[string]interface{}{"action": action} // Создаём val с действием.

	switch action { // Проверяем действие.
	case "submit": // Отправка ответа.
		if choice, ok := cmd.Payload["choice"].(int); ok { // Если есть choice.
			val["choice"] = choice // Устанавливаем choice.
		} // Конец проверки choice.
	case "select", "unselect": // Выбор/отмена варианта (final round).
		if choice, ok := cmd.Payload["choice"].(int); ok { // Если есть choice.
			val["choice"] = choice // Устанавливаем choice.
		} // Конец проверки choice.
	case "select_grid": // Выбор ячейки в сетке (шпаги).
		if x, ok := cmd.Payload["x"].(int); ok { // Если есть x.
			val["x"] = x // Устанавливаем x.
		} // Конец проверки x.
		if y, ok := cmd.Payload["y"].(int); ok { // Если есть y.
			val["y"] = y // Устанавливаем y.
		} // Конец проверки y.
	} // Конец switch.

	message := map[string]interface{}{ // Создаём карту для сообщения.
		"seq":    1,              // Порядковый номер сообщения.
		"opcode": "object/update", // Код операции для player.
		"params": map[string]interface{}{ // Параметры сообщения.
			"key": responseKey, // Ключ ответа.
			"val": val,         // Значение с действием.
		}, // Конец параметров.
	} // Конец создания сообщения.

	return message, nil // Возвращаем сообщение без ошибки.
} // Конец buildTD2PlayerMessage.

// buildTD2AudienceMessage строит сообщение audience/count-group/increment для обычного раунда TriviaDeath2.
// Поддерживает как новый формат (triviadeath2), так и старый (triviadeath2-tjsp).
func buildTD2AudienceMessage(cmd ClientCommand) (map[string]interface{}, error) { // Построение сообщения audience.
	gameTag, _ := cmd.Payload["gameTag"].(string)                          // Тег игры.
	isNewFormat, _ := cmd.Payload["isNewFormat"].(bool)                    // Флаг нового формата.

	if isNewFormat && gameTag != "triviadeath2-tjsp" && strings.Contains(gameTag, "triviadeath2") { // Новый формат (triviadeath2, не tjsp).
		answerKey, ok := cmd.Payload["answerKey"].(string) // Ключ ответа из payload.
		if !ok { // Если ключ не найден в payload.
			answerKey = cmd.Answer // Используем Answer как ключ.
		} // Конец проверки ключа.

		message := map[string]interface{}{ // Создаём карту для сообщения.
			"seq":    1,                                // Порядковый номер сообщения.
			"opcode": "audience/count-group/increment", // Код операции для подтверждения ответа зрителя.
			"params": map[string]interface{}{ // Параметры сообщения.
				"name":  "TriviaDeath2 Vote", // Имя группы подсчёта (новый формат).
				"vote":  answerKey,           // Ключ выбранного ответа.
				"times": 1,                   // Количество раз (всегда 1).
			}, // Конец параметров.
		} // Конец создания сообщения.

		return message, nil // Возвращаем сообщение без ошибки.
	} // Конец проверки нового формата.

	// Старый формат (triviadeath2-tjsp): используем числовой индекс ответа.
	answerIndex, ok := cmd.Payload["answerIndex"].(int) // Получаем индекс ответа.
	if !ok { // Если индекс не найден.
		if answerStr := cmd.Answer; answerStr != "" { // Если есть строка ответа.
			if idx, err := strconv.Atoi(answerStr); err == nil { // Если удалось преобразовать в число.
				answerIndex = idx // Устанавливаем индекс.
			} else { // Если преобразование не удалось.
				return nil, fmt.Errorf("invalid answer index for Trivia Death 2: %s", answerStr) // Возвращаем ошибку.
			} // Конец проверки преобразования.
		} else { // Если строки ответа нет.
			return nil, fmt.Errorf("answer index not found for Trivia Death 2") // Возвращаем ошибку.
		} // Конец проверки строки ответа.
	} // Конец проверки индекса.

	message := map[string]interface{}{ // Создаём карту для сообщения.
		"seq":    1,                                // Порядковый номер сообщения.
		"opcode": "audience/count-group/increment", // Код операции для подтверждения ответа зрителя.
		"params": map[string]interface{}{ // Параметры сообщения.
			"name":  "TriviaDeath2AudienceChoice",   // Имя группы подсчёта (старый формат).
			"vote":  fmt.Sprintf("%d", answerIndex), // Индекс выбранного ответа (в виде строки).
			"times": 1,                              // Количество раз (всегда 1).
		}, // Конец параметров.
	} // Конец создания сообщения.

	return message, nil // Возвращаем сообщение без ошибки.
} // Конец buildTD2AudienceMessage.

// buildTD2FinalRoundMessage строит сообщение audience/count-group/increment для финального раунда TriviaDeath2.
// Поддерживает как новый формат (triviadeath2), так и старый (triviadeath2-tjsp).
func buildTD2FinalRoundMessage(cmd ClientCommand) (map[string]interface{}, error) { // Построение сообщения финального раунда.
	gameTag, _ := cmd.Payload["gameTag"].(string)   // Тег игры.
	isNewFormat, _ := cmd.Payload["isNewFormat"].(bool) // Флаг нового формата.

	voteString := cmd.Answer // Индексы выбранных ответов через запятую (например, "0,1" или "1,2").

	// Определяем имя группы подсчёта в зависимости от формата.
	name := "TriviaDeath2AudienceChoice" // Имя по умолчанию (старый формат).
	if isNewFormat && gameTag != "triviadeath2-tjsp" && strings.Contains(gameTag, "triviadeath2") { // Новый формат.
		name = "TriviaDeath2 Vote" // Имя группы для нового формата.
	} // Конец определения имени.

	message := map[string]interface{}{ // Создаём карту для сообщения.
		"seq":    1,                                // Порядковый номер сообщения.
		"opcode": "audience/count-group/increment", // Код операции для подтверждения ответа зрителя.
		"params": map[string]interface{}{ // Параметры сообщения.
			"name":  name,        // Имя группы подсчёта.
			"vote":  voteString,  // Индексы выбранных ответов через запятую.
			"times": 1,           // Количество раз (всегда 1).
		}, // Конец параметров.
	} // Конец создания сообщения.

	return message, nil // Возвращаем сообщение без ошибки.
} // Конец buildTD2FinalRoundMessage.

// buildDefaultMessage строит сообщение для всех остальных игр (Poll Position, generic и т.д.).
// Для Poll Position формирует audience/count-group/increment.
// Для остальных — общий формат с type/eventId/answer.
func buildDefaultMessage(cmd ClientCommand) (map[string]interface{}, error) { // Построение сообщения по умолчанию.
	gameTag, _ := cmd.Payload["gameTag"].(string) // Тег игры.

	if gameTag == "pollposition" { // Если это Poll Position.
		vote, _ := cmd.Payload["vote"].(string)     // Vote ("0" или "1").
		opcode, _ := cmd.Payload["opcode"].(string) // Код операции.
		name, _ := cmd.Payload["name"].(string)     // Имя группы подсчёта.

		if vote == "" { // Если vote не найден в payload.
			vote = cmd.Answer // Используем Answer как vote.
		} // Конец проверки vote.

		if opcode == "" { // Если opcode не найден.
			opcode = "audience/count-group/increment" // Устанавливаем opcode по умолчанию.
		} // Конец проверки opcode.

		if name == "" { // Если name не найден.
			name = "Poll Position Vote" // Устанавливаем name по умолчанию.
		} // Конец проверки name.

		message := map[string]interface{}{ // Создаём карту для сообщения.
			"seq":    1,      // Порядковый номер сообщения (начинаем с 1).
			"opcode": opcode, // Код операции для Poll Position.
			"params": map[string]interface{}{ // Параметры сообщения.
				"name":  name, // Имя группы подсчёта.
				"vote":  vote, // Vote ("0" или "1").
				"times": 1,    // Количество раз (всегда 1).
			}, // Конец параметров.
		} // Конец создания сообщения.

		return message, nil // Возвращаем сообщение без ошибки.
	} // Конец проверки Poll Position.

	// Общий формат для остальных игр.
	message := map[string]interface{}{ // Создаём карту для сообщения.
		"type":    cmd.Type,    // Устанавливаем тип команды.
		"eventId": cmd.EventID, // Устанавливаем ID события.
		"answer":  cmd.Answer,  // Устанавливаем ответ.
	} // Конец создания сообщения.

	for k, v := range cmd.Payload { // Добавляем дополнительные данные из payload.
		message[k] = v // Добавляем данные в сообщение.
	} // Конец цикла.

	return message, nil // Возвращаем сообщение без ошибки.
} // Конец buildDefaultMessage.

// generateRandomAudienceName генерирует случайное имя для зрителя.
// Возвращает строку из 4 заглавных букв (например, "AUDI").
func generateRandomAudienceName() string { // Функция генерации случайного имени зрителя.
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" // Набор заглавных букв для генерации имени.
	const nameLength = 4                         // Длина имени (4 символа).

	b := make([]byte, nameLength) // Создаём слайс байт для случайных символов.
	rand.Read(b)                  // Заполняем слайс случайными байтами.

	result := make([]byte, nameLength) // Создаём слайс для результата.
	for i := range b {                 // Проходим по каждому байту.
		result[i] = charset[b[i]%byte(len(charset))] // Преобразуем байт в символ из charset.
	} // Конец цикла.

	return string(result) // Возвращаем строку из 4 заглавных букв.
} // Конец generateRandomAudienceName.

// tryLearnFromMessage проверяет, содержит ли сообщение правильный ответ, и сохраняет его.
// Принимает сырые байты WebSocket сообщения и менеджер ботнета.
func tryLearnFromMessage(raw []byte, manager *BotnetManager) { // Функция обучения из сообщения.
	// Парсим сообщение как JSON.
	var msg struct { // Структура сообщения.
		Result struct { // Результат.
			Key string `json:"key"` // Ключ (должен быть "textDescriptions").
			Val struct { // Значение.
				LatestDescriptions []struct { // Описания.
					Category string `json:"category"` // Категория.
					Text     string `json:"text"`     // Текст.
				} `json:"latestDescriptions"` // Массив описаний.
			} `json:"val"` // Значение.
		} `json:"result"` // Результат.
	} // Конец структуры.

	if err := json.Unmarshal(raw, &msg); err != nil { // Если не удалось распарсить.
		return // Выходим (не наш формат).
	} // Конец парсинга.

	if msg.Result.Key != "textDescriptions" { // Если это не textDescriptions.
		return // Выходим.
	} // Конец проверки ключа.

	log.Printf("[JF-LEARN] received textDescriptions message, categories count: %d", len(msg.Result.Val.LatestDescriptions)) // Логируем получение.

	// Получаем текущий вопрос.
	manager.mu.RLock() // Блокируем мьютекс для чтения.
	q := manager.currentQuestion // Получаем текущий вопрос.
	manager.mu.RUnlock() // Разблокируем мьютекс.

	if q == nil { // Если нет текущего вопроса.
		log.Printf("[JF-LEARN] no current question set, skipping") // Логируем пропуск.
		return // Выходим.
	} // Конец проверки вопроса.

	log.Printf("[JF-LEARN] current question: %s, processing answers...", q.Prompt[:min(60, len(q.Prompt))]) // Логируем текущий вопрос.

	// Ищем правильный ответ в описания.
	for _, desc := range msg.Result.Val.LatestDescriptions { // Проходим по описаниям.
		var answerTexts []string // Тексты правильных ответов.

		if desc.Category == "TEXT_DESCRIPTION_CORRECT_ANSWER" { // Если одиночный ответ.
			// "Верный ответ: X" → извлекаем X.
			prefix := "Верный ответ: " // Префикс.
			if idx := strings.Index(desc.Text, prefix); idx >= 0 { // Если префикс найден.
				answerTexts = []string{desc.Text[idx+len(prefix):]} // Извлекаем ответ.
			} // Конец извлечения.
		} else if desc.Category == "TEXT_DESCRIPTION_CORRECT_ANSWERS" { // Если несколько ответов.
			// "Верные ответы: X и Y" → извлекаем X, Y.
			prefix := "Верные ответы: " // Префикс.
			if idx := strings.Index(desc.Text, prefix); idx >= 0 { // Если префикс найден.
				rest := desc.Text[idx+len(prefix):] // Остаток строки.
				answerTexts = strings.Split(rest, " и ") // Разбиваем по " и ".
			} // Конец извлечения.
		} // Конец проверки категории.

		if len(answerTexts) == 0 { // Если ответы не найдены.
			continue // Продолжаем.
		} // Конец проверки.

		// Сопоставляем тексты ответов с индексами в choices.
		answers := []server.QuestionAnswer{} // Ответы для сохранения.
		for _, text := range answerTexts { // Проходим по текстам ответов.
			text = strings.TrimSpace(text) // Убираем пробелы.
			for i, choice := range q.Choices { // Проходим по вариантам.
				if choice == text { // Если текст совпадает.
					answers = append(answers, server.QuestionAnswer{Text: text, Index: i}) // Добавляем ответ.
					break // Прерываем внутренний цикл.
				} // Конец проверки совпадения.
			} // Конец цикла по вариантам.
		} // Конец цикла по текстам.

		if len(answers) > 0 { // Если нашли ответы.
			log.Printf("coordinator: learned %d answers for: %s", len(answers), q.Prompt[:min(60, len(q.Prompt))]) // Логируем обучение.
			saveQuestionToFile(q.Prompt, answers) // Сохраняем в файл.
		} // Конец проверки.
	} // Конец цикла по описаниям.
} // Конец tryLearnFromMessage.

// saveQuestionToFile сохраняет вопрос с ответами в questions.json.
// Принимает текст вопроса и слайс ответов.
func saveQuestionToFile(prompt string, answers []server.QuestionAnswer) { // Функция сохранения вопроса в файл.
	const questionsFile = "questions.json" // Путь к файлу банка вопросов.

	// Загружаем существующий банк.
	bank := server.QuestionBank{} // Банк вопросов.
	data, readErr := os.ReadFile(questionsFile) // Читаем файл.
	if readErr == nil { // Если файл существует.
		_ = json.Unmarshal(data, &bank) // Игнорируем ошибку парсинга.
	} // Конец загрузки.

	// Ищем существующий вопрос по prompt.
	now := time.Now().UnixMilli() // Текущее время.
	found := false               // Флаг нахождения.
	for i, q := range bank.Questions { // Проходим по вопросам.
		if q.Prompt == prompt { // Если prompt совпадает.
			bank.Questions[i].SeenCount++ // Увеличиваем счётчик.
			bank.Questions[i].LastSeen = now // Обновляем время.
			// Добавляем новые ответы.
			for _, newAns := range answers { // Проходим по новым ответам.
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
		bank.Questions = append(bank.Questions, server.QuestionEntry{ // Добавляем.
			Prompt:    prompt,    // Текст вопроса.
			Answers:   answers,   // Ответы.
			SeenCount: 1,         // Первое появление.
			LastSeen:  now,       // Текущее время.
		}) // Конец добавления.
	} // Конец проверки.

	// Сохраняем банк.
	data, err := json.MarshalIndent(bank, "", "  ") // Форматируем JSON.
	if err != nil { // Если форматирование не удалось.
		log.Printf("coordinator: failed to marshal questions: %v", err) // Логируем ошибку.
		return // Выходим.
	} // Конец форматирования.

	if err := os.WriteFile(questionsFile, data, 0o644); err != nil { // Записываем файл.
		log.Printf("coordinator: failed to write questions.json: %v", err) // Логируем ошибку.
		return // Выходим.
	} // Конец записи.

	log.Printf("coordinator: saved question to %s (total: %d)", questionsFile, len(bank.Questions)) // Логируем сохранение.
} // Конец saveQuestionToFile.

// min возвращает минимальное из двух чисел.
func min(a, b int) int { // Функция минимума.
	if a < b { // Если a меньше b.
		return a // Возвращаем a.
	} // Конец проверки.
	return b // Возвращаем b.
} // Конец min.
