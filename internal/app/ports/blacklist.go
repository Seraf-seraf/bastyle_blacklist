package ports

type BlacklistStore interface {
	Block(fileUniqueID string)
	IsBlocked(fileUniqueID string) bool
}
