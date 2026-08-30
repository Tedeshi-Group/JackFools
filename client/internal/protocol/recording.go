package protocol // Пакет protocol описывает структуры обмена между расширением и клиентом.

import ( // Начинаем блок импортов.
	"errors"  // Возвращаем понятные ошибки валидации.
	"strings" // Проверяем строковые поля.
) // Конец импортов.

// RecordingMessage представляет один перехваченный WebSocket сообщение.
type RecordingMessage struct { // Структура сообщения.
	Dir  string `json:"dir"`  // Направление: "send" или "recv".
	TS   int64  `json:"ts"`   // Unix timestamp в миллисекундах.
	Data string `json:"data"` // Данные WebSocket сообщения в виде строки.
} // Конец RecordingMessage.

// Recording представляет записанную сессию WebSocket активности.
type Recording struct { // Структуры записи.
	ActionName string             `json:"action_name"` // Имя действия (обязательно).
	Note       string             `json:"note"`        // Примечание (опционально).
	PageURL    string             `json:"page_url"`    // URL страницы.
	StartedAt  int64              `json:"started_at"`  // Timestamp начала записи.
	StoppedAt  int64              `json:"stopped_at"`  // Timestamp окончания записи.
	Messages   []RecordingMessage `json:"messages"`     // Массив перехваченных сообщений.
} // Конец Recording.

// Validate проверяет, что запись содержит все обязательные поля.
func (r Recording) Validate() error { // Функция валидации записи.
	if strings.TrimSpace(r.ActionName) == "" { // Имя действия обязательно.
		return errors.New("action_name is required") // Возвращаем ошибку.
	} // Конец проверки имени действия.
	if r.StartedAt == 0 { // Timestamp начала обязателен.
		return errors.New("started_at is required") // Возвращаем ошибку.
	} // Конец проверки started_at.
	if r.StoppedAt == 0 { // Timestamp окончания обязателен.
		return errors.New("stopped_at is required") // Возвращаем ошибку.
	} // Конец проверки stopped_at.
	if len(r.Messages) == 0 { // Массив сообщений не может быть пустым.
		return errors.New("messages must be a non-empty array") // Возвращаем ошибку.
	} // Конец проверки сообщений.
	for i, m := range r.Messages { // Проходим по каждому сообщению.
		if m.Dir != "send" && m.Dir != "recv" { // Направление должно быть send или recv.
			return errors.New("messages[" + itoa(i) + "].dir must be \"send\" or \"recv\"") // Возвращаем ошибку.
		} // Конец проверки направления.
		if m.TS == 0 { // Timestamp сообщения обязателен.
			return errors.New("messages[" + itoa(i) + "].ts must be > 0") // Возвращаем ошибку.
		} // Конец проверки timestamp.
		if m.Data == "" { // Данные сообщения обязательны.
			return errors.New("messages[" + itoa(i) + "].data is required") // Возвращаем ошибку.
		} // Конец проверки данных.
	} // Конец цикла по сообщениям.
	return nil // Всё ок.
} // Конец Validate.

// itoa преобразует неотрицательное число в строку без импорта strconv.
func itoa(n int) string { // Функция преобразования числа в строку.
	if n == 0 { // Если число равно нулю.
		return "0" // Возвращаем "0".
	} // Конец проверки нуля.
	buf := [20]byte{}       // Буфер для цифр (максимум 20 цифр для int64).
	i := len(buf)           // Начинаем с конца буфера.
	for n > 0 {             // Пока есть цифры.
		i--                      // Сдвигаем позицию влево.
		buf[i] = byte('0' + n%10) // Записываем последнюю цифру.
		n /= 10                  // Убираем последнюю цифру.
	} // Конец цикла.
	return string(buf[i:]) // Возвращаем строку.
} // Конец itoa.
