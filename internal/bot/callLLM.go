package bot

import "google.golang.org/genai"

func Call(history []*genai.Content) (*genai.GenerateContentResponse, error) {
	result, err := gclient.Models.GenerateContent(
		ctx,
		botConfig.Model.Model,
		history,
		botConfig.Model.Config,
	)
	return result, err
}
