package protocol // Пакет protocol описывает структуры обмена между расширением и клиентом.

import ( // Начинаем блок импортов.
	"errors"  // Возвращаем понятные ошибки валидации.
	"strings" // Проверяем строковые поля.
) // Конец импортов.

type Event struct { // Событие от расширения к клиенту.
	Type    string                 `json:"type"`    // Тип события (например "page_state").
	URL     string                 `json:"url"`     // URL текущей страницы игры.
	TS      int64                  `json:"ts"`      // Unix timestamp (ms или s — не критично на старте).
	Payload map[string]any         `json:"payload"` // Полезная нагрузка (снимок состояния).
} // Конец Event.

func (e Event) Validate() error { // Валидация входного события.
	if strings.TrimSpace(e.Type) == "" { // Type обязателен.
		return errors.New("type is required") // Возвращаем ошибку.
	} // Конец проверки type.

	if strings.TrimSpace(e.URL) == "" { // URL обязателен.
		return errors.New("url is required") // Возвращаем ошибку.
	} // Конец проверки url.

	if e.Payload == nil { // Payload должен быть объектом (map), а не null.
		return errors.New("payload is required") // Возвращаем ошибку.
	} // Конец проверки payload.

	return nil // Всё ок.
} // Конец Validate.




