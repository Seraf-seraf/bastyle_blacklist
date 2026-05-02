package ports

type AdminChecker interface {
	IsAdmin(chatID int64, userID int64) (bool, error)
}
