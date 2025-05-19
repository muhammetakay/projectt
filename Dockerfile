# Golang image'i seçiyoruz
FROM golang:1.24.3-alpine

# Proje dizinini oluştur
WORKDIR /app

# Modül dosyalarını kopyala ve bağımlılıkları yükle
COPY go.mod go.sum ./
RUN go mod download

# Tüm dosyaları kopyala
COPY . .

# Uygulamayı derle
RUN go build -o /main

# Uygulamayı çalıştır
CMD ["/main"]