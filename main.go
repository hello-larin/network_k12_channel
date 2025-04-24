package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

// Структура для представления сегмента
type Segment struct {
	Segment       string `json:"segment"`
	SegmentNumber int    `json:"segment_number"`
	SendTime      string `json:"send_time"`
	TotalSegments int    `json:"total_segments"`
	UserID        string `json:"username"`
}

// Создаём генератор случайных чисел
var randomGenerator *rand.Rand

// Вызывается до main 1 раз - инициализирует генератор
func init() {
	randomGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Кодирование кода Хэмминга 15,11
func encodeHamming(data []byte) []byte {
	// Добавляем контрольные биты на позиции 1, 2, 4, 8
	// Из-за того что в GO начало массива идёт с 0 пропустим
	// 1 ячейку
	code := make([]byte, 16)
	j := 0
	for i := range 15 {
		if i == 0 || i == 1 || i == 3 || i == 7 {
			continue
		} else {
			code[i] = data[j]
			j += 1
		}
	}

	// Вычисляем контрольные биты
	// 1 = 3 + 5 + 7 + 9 + 11 + 13 + 15 --> 0 = 2 + 4 + 6 + 8 + 10 + 12 + 14
	// 2 = 3 + 6 + 7 + 10 + 11 + 14 + 15 --> 1 = 2 + 5 + 6 + 9 + 10 + 13 + 14
	// 4 = 5 + 6 + 7 + 12 + 13 + 14 + 15 --> 3 = 4 + 5 + 6 + 11 + 12 + 13 + 14
	// 8 = 9 + 10 + 11 + 12 + 13 + 14 + 15 --> 7 = 8 + 9 + 10 + 11 + 12 + 13 + 14
	code[0] = (code[2] + code[4] + code[6] + code[8] + code[10] + code[12] + code[14]) % 2
	code[1] = (code[2] + code[5] + code[6] + code[9] + code[10] + code[13] + code[14]) % 2
	code[3] = (code[4] + code[5] + code[6] + code[11] + code[12] + code[13] + code[14]) % 2
	code[7] = (code[8] + code[9] + code[10] + code[11] + code[12] + code[13] + code[14]) % 2

	// Добавляем дополнительный бит для обнаружения ошибки рода даже если исправили
	// Это сумма всех битов
	var finalByte byte = 0
	for _, b := range code {
		finalByte += b
	}
	code[15] = finalByte % 2

	return code
}

// Декодирование кода Хэмминга 15,11
func decodeHamming(code []byte) ([]byte, error) {
	// Вычисляем синдром
	syndrome := make([]byte, 4)
	syndrome[0] = (code[0] + code[2] + code[4] + code[6] + code[8] + code[10] + code[12] + code[14]) % 2
	syndrome[1] = (code[1] + code[2] + code[5] + code[6] + code[9] + code[10] + code[13] + code[14]) % 2
	syndrome[2] = (code[3] + code[4] + code[5] + code[6] + code[11] + code[12] + code[13] + code[14]) % 2
	syndrome[3] = (code[7] + code[8] + code[9] + code[10] + code[11] + code[12] + code[13] + code[14]) % 2

	// Преобразуем синдром в число
	errorPosition := syndrome[3]*8 + syndrome[2]*4 + syndrome[1]*2 + syndrome[0]

	var finalByte byte = 0
	for _, b := range code {
		finalByte += b
	}
	finalByte = finalByte % 2

	// Проверяем наличие двойной ошибки
	if errorPosition > 0 && finalByte == 0 {
		return nil, errors.New("two errors detected")
	}

	// Проверяем наличие одиночной ошибки
	if errorPosition > 0 && errorPosition <= 15 {
		errorPosition -= 1 // Уменьшаем позицию на 1
		code[errorPosition] = 1 - code[errorPosition]
	}

	// Извлекаем данные
	data := make([]byte, 11)
	j := 0
	for i := 1; i < 15; i++ {
		if i != 0 && i != 1 && i != 3 && i != 7 {
			data[j] = code[i]
			j++
		}
	}

	return data, nil
}

// Внесение ошибки
func introduceErrors(code []byte) []byte {
	// Вероятность ошибки 7%
	// -- Вероятность однократной 75%
	// -- Вероятность двухкратной 25%
	if randomGenerator.Float64() < 0.07 {
		if randomGenerator.Float64() < 0.95 {
			pos1 := randomGenerator.Intn(15)
			code[pos1] = 1 - code[pos1]
		} else {
			fmt.Println("TWO ERROR")
			pos1 := randomGenerator.Intn(15)
			pos2 := randomGenerator.Intn(15)
			code[pos1] = 1 - code[pos1]
			code[pos2] = 1 - code[pos2]
		}
	}
	return code
}

func strToBinary(s string) []byte {
	var b []byte

	for _, c := range s {
		if c == '1' {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}

	return b
}

func split(data []byte) [][]byte {
	var result [][]byte

	// Проходим по строке с шагом 11 бит
	for i := 0; i < len(data); i += 11 {
		end := min(i+11, len(data))
		// Делаем срез из 11 битов и кодируем Хэммингом
		block := make([]byte, 11)
		copy(block, data[i:end])
		// Добавляем в финальный массив
		result = append(result, block)
	}

	return result
}

// Обработка POST-запроса
func handleCode(w http.ResponseWriter, r *http.Request) {
	var segment Segment
	err := json.NewDecoder(r.Body).Decode(&segment)
	if err != nil {
		fmt.Println("Invalid JSON format")
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Потяра кадра 1%
	if randomGenerator.Float64() < 0.01 {
		fmt.Println("Packet lost")
		http.Error(w, "Packet lost", http.StatusInternalServerError)
		return
	}

	// Кодирование строки в массив битов
	dataBits := strToBinary(segment.Segment)

	// Разбиение
	data := split(dataBits)

	for i, value := range data {
		// Кодирование Хэммингом
		data[i] = encodeHamming(value)
		data[i] = introduceErrors(data[i])
		decoded, err := decodeHamming(data[i])
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		copy(dataBits[i*11:], decoded)
	}

	w.WriteHeader(http.StatusOK)

	// Затем отправляем декодированный сегмент на localhost:8002/transfer
	// Используем горутину, чтобы не блокировать ответ клиенту
	go sendToTransfer(segment, dataBits)
}

// Функция для отправки данных на другой сервер
func sendToTransfer(segment Segment, dataBits []byte) {
	// Конвертируем биты обратно в строку
	for i, c := range dataBits {
		if c == 1 {
			dataBits[i] = '1'
		} else {
			dataBits[i] = '0'
		}
	}

	segment.Segment = string(dataBits)

	jsonData, err := json.Marshal(segment)
	if err != nil {
		fmt.Println("Failed to marshal segment:", err)
		return
	}

	fmt.Println("DATA TRANSFER", string(jsonData))

	resp, err := http.Post("http://localhost:8002/transfer", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Failed to send data to localhost:8002/transfer:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Unexpected status code from localhost:8002/transfer: %d, response: %s\n", resp.StatusCode, body)
	} else {
		fmt.Println("Data successfully sent to localhost:8002/transfer")
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /code", func(w http.ResponseWriter, r *http.Request) {
		handleCode(w, r)
	})
	fmt.Println("SERVER STARTED")

	http.ListenAndServe("localhost:8001", mux)
}
