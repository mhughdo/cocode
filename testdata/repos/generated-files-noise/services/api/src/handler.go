package src

func UpdateBillingLimit(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}
