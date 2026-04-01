package billing

type ConsumeToken struct {
	Day   Token `json:"Day"`
	Month Token `json:"Month"`
	Year  Token `json:"Year"`
}

func (c *ConsumeToken) CountTotal() int64 {
	return c.Day.CountTotal() + c.Month.CountTotal() + c.Year.CountTotal()
}
