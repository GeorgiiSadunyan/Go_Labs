# Используем официальный образ Go
FROM golang:1.23-alpine

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем файлы зависимостей и скачиваем их
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение (файл будет называться main)
RUN go build -o main cmd/main.go

# Открываем порт 8080
EXPOSE 8080

# Запускаем
CMD ["./main"]