package aivector

import "context"

type Client interface {
	// Embed отправляет кадры в AI-сервис и получает embeddings без записи ban-а в БД и без обновления FAISS.
	// Результат используется transactional /ban, где bot сохраняет vectors в общей PostgreSQL-транзакции.
	Embed(ctx context.Context, request EmbedRequest) (EmbedResponse, error)

	// Search отправляет кадры в AI-сервис для поиска по текущему локальному FAISS-индексу.
	// Метод не изменяет состояние и используется при проверке входящих медиа.
	Search(ctx context.Context, request SearchRequest) (SearchResponse, error)
}

type FrameFile struct {
	Name string
	Data []byte
}
