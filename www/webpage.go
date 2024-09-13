package www

type WebPage struct {
	Title   string         `json:"title"`
	Content map[string]any `json:"content"`
}

func NewWebPage(title string, content map[string]any) *WebPage {
	return &WebPage{Title: title, Content: content}
}
