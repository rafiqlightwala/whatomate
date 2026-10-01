package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/shridarpatil/whatomate/internal/builtin"
)

type Message struct {
	ID        string `json:"id"`
	Direction string `json:"direction"` // incoming or outgoing
	Text      string `json:"text"`
	Media     string `json:"media"` // metadata only; never implies media was read
	Pending   bool   `json:"pending"`
}

type Question struct {
	Category       string   `json:"category"`
	Question       string   `json:"question"`
	MessageIDs     []string `json:"message_ids"`
	AttemptedSteps []string `json:"attempted_steps"`
}

type Decision struct {
	Decision  string     `json:"decision"`
	Reason    string     `json:"reason"`
	Language  string     `json:"language"`
	Summary   string     `json:"summary"`
	Questions []Question `json:"questions"`
}

// Generate uses the configured provider. Callers must honor ctx cancellation,
// keep credentials out of transcripts, and disable legacy session history.
type Generate func(ctx context.Context, system, input string) (string, error)

type Prepared struct {
	Decision
	Version  string `json:"version"`
	Body     string `json:"body"`
	Fallback bool   `json:"fallback"`
}

// Prepare classifies first, then writes one reply from approved product facts.
// It cannot reserve budget or send. The worker must validate the conversation
// revision, manual suppression and service window again after this returns.
func Prepare(ctx context.Context, messages []Message, generate Generate) (Prepared, error) {
	result := Prepared{Version: builtin.InvestifySupportVersion}
	if err := validateMessages(messages); err != nil {
		return result, err
	}
	if allNoise(messages) {
		result.Decision = Decision{Decision: "skip", Reason: "No substantive pending message", Language: "en", Questions: []Question{}}
		return result, nil
	}
	if generate == nil {
		return result, errors.New("support AI generator is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	input, err := json.Marshal(struct {
		Messages []Message `json:"messages"`
	}{messages})
	if err != nil {
		return result, err
	}
	raw, err := generate(ctx, builtin.InvestifyQueuePrompt, string(input))
	if err != nil {
		return result, fmt.Errorf("classify support conversation: %w", err)
	}
	result.Decision, err = ParseDecision(raw, messages)
	if err != nil {
		return result, err // Retry classification, never interpret invalid JSON as permission to send.
	}
	if result.Decision.Decision == "skip" {
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	answerInput, err := json.Marshal(struct {
		Decision Decision  `json:"classification"`
		Messages []Message `json:"messages"`
	}{result.Decision, messages})
	if err != nil {
		return result, err
	}
	body, answerErr := generate(ctx, builtin.InvestifyAnswerPrompt+"\n\nAPPROVED KNOWLEDGE\n"+builtin.LoadInvestifyAIContextSummary(), string(answerInput))
	if err := ctx.Err(); err != nil {
		return result, err
	}
	body = CleanEmailHandoff(body)
	if answerErr != nil || body == "" || !utf8.ValidString(body) || utf8.RuneCountInString(body) > MaxBodyRunes {
		// Intent has already been established. Preserve the one-message policy
		// using a complete email handoff rather than truncating useful advice.
		body = fallback(result.Language)
		result.Fallback = true
	}
	result.Body = body + "\n\n" + footer(result.Language)
	if utf8.RuneCountInString(result.Body) > MaxReplyRunes {
		return Prepared{}, errors.New("support reply exceeds application limit")
	}
	return result, nil
}

func validateMessages(messages []Message) error {
	seen := map[string]bool{}
	total := 0
	for _, m := range messages {
		if m.ID == "" || seen[m.ID] || (m.Direction != "incoming" && m.Direction != "outgoing") || (m.Pending && m.Direction != "incoming") {
			return errors.New("invalid support transcript metadata")
		}
		if !utf8.ValidString(m.Text) {
			return errors.New("invalid support transcript text")
		}
		seen[m.ID] = true
		total += len(m.Text)
	}
	// Never silently drop earlier unanswered questions to fit a token budget.
	if total > 64000 || len(messages) > 200 {
		return errors.New("support transcript needs compaction before classification")
	}
	return nil
}

func allNoise(messages []Message) bool {
	for _, m := range messages {
		if m.Pending && m.Direction == "incoming" && !ObviousNoise(m.Text, m.Media) {
			return false
		}
	}
	return true
}

// ParseDecision rejects unsupported fields, values and invented evidence IDs.
// Semantic accuracy still needs fixture evaluation against the configured model.
func ParseDecision(raw string, messages []Message) (Decision, error) {
	var d Decision
	if len(raw) > 32000 {
		return d, errors.New("support classification is too large")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, fmt.Errorf("invalid support classification: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return d, errors.New("support classification contains trailing output")
	}
	if d.Decision != "answer" && d.Decision != "skip" && d.Decision != "email" {
		return d, errors.New("invalid support decision")
	}
	if d.Language != "en" && d.Language != "roman_ur" && d.Language != "ur" {
		return d, errors.New("invalid support language")
	}
	if strings.TrimSpace(d.Reason) == "" || (d.Decision != "skip" && strings.TrimSpace(d.Summary) == "") || d.Questions == nil || len(d.Questions) > 32 {
		return d, errors.New("incomplete support classification")
	}
	if (d.Decision == "skip" && len(d.Questions) != 0) || (d.Decision == "answer" && len(d.Questions) == 0) {
		return d, errors.New("support decision contradicts its questions")
	}
	validIDs := map[string]bool{}
	for _, m := range messages {
		if m.Direction == "incoming" && m.Pending {
			validIDs[m.ID] = true
		}
	}
	if d.Decision != "skip" && len(validIDs) == 0 {
		return d, errors.New("support decision has no pending customer evidence")
	}
	for _, q := range d.Questions {
		switch q.Category {
		case "account", "web", "portfolio", "market_data", "subscription", "ads", "brokerage", "feedback", "other":
		default:
			return d, errors.New("invalid support category")
		}
		if strings.TrimSpace(q.Question) == "" || len(q.MessageIDs) == 0 {
			return d, errors.New("support question is missing evidence")
		}
		for _, id := range q.MessageIDs {
			if !validIDs[id] {
				return d, errors.New("support question cites nonpending or unknown evidence")
			}
		}
	}
	return d, nil
}

func footer(language string) string {
	switch language {
	case "roman_ur":
		return "WhatsApp support ke kharch ko manageable rakhne ke liye automated jawab mahine mein ek baar diya jata hai. Mazeed madad ya sawalon ke liye support@investify.pk par email karein."
	case "ur":
		return "واٹس ایپ سپورٹ کے اخراجات محدود رکھنے کے لیے خودکار جواب مہینے میں ایک بار دیا جاتا ہے۔ مزید مدد یا سوالات کے لیے support@investify.pk پر ای میل کریں۔"
	default:
		return "To keep WhatsApp support sustainable, automated replies are limited to one per month. For further help or questions, please email support@investify.pk."
	}
}

func fallback(language string) string {
	switch language {
	case "roman_ur":
		return "Is maslay ka mukammal jawab yahan tayyar nahi ho saka. Email mein apna sawal, istemal hone wala platform (web, Android ya iOS), aur zaroori error ya screenshot shamil karein. Password ya verification code na bhejein."
	case "ur":
		return "اس مسئلے کا مکمل جواب یہاں تیار نہیں ہو سکا۔ ای میل میں اپنا سوال، استعمال ہونے والا پلیٹ فارم (ویب، اینڈرائیڈ یا آئی او ایس) اور متعلقہ خرابی یا اسکرین شاٹ شامل کریں۔ پاس ورڈ یا تصدیقی کوڈ نہ بھیجیں۔"
	default:
		return "We could not prepare a complete answer here. Please include your question, the platform you use (web, Android or iOS), and any relevant error or screenshot in your email. Do not send passwords or verification codes."
	}
}

// CleanEmailHandoff also protects queued replies prepared before the owner
// removed the public email landing page from WhatsApp messages.
func CleanEmailHandoff(body string) string {
	for _, url := range []string{"https://wa.recubetech.com/support/email", "http://wa.recubetech.com/support/email"} {
		body = strings.ReplaceAll(body, url, "support@investify.pk")
	}
	return strings.TrimSpace(body)
}
