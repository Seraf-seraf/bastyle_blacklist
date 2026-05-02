package ports

type BlacklistStore interface {
	Block(fileUniqueID string) error
	IsBlocked(fileUniqueID string) (bool, error)
}
