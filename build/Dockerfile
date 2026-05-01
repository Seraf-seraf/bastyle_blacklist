# Базовый образ для Go
FROM golang:latest

# Создание директории
RUN mkdir /app

# Копирование файлов в директорию app
ADD .. /app/

# Установка рабочей директории
WORKDIR /app

# Получение зависимостей
RUN go get -d

# Сборка приложения
RUN go build /app/cmd/main.go -o main

# Запуск бота
CMD ["/app/main"]