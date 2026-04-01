package billing

type Token struct {
	Input  int64 `json:"Input"`
	Output int64 `json:"Output"`
	Think  int64 `json:"Think"`
}

func (t *Token) Add(input, output, think int32) {
	t.Input += int64(input)
	t.Output += int64(output)
	t.Think += int64(think)
}

func (t *Token) ResetTokenUsage() {
	t.Input = 0
	t.Output = 0
	t.Think = 0
}

func (t *Token) CountTotal() int64 {
	return t.Input + t.Output + t.Think
}
