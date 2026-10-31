package jev

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"os"
	"regexp"
	"strings"

	"github.com/cristianadrielbraun/gofer/internal/storage"
)

const (
	maxRawMessageBytes = 16 << 20
	maxBodyCharacters  = 12000
	maxAttachments     = 50
	maxMIMEParts       = 100
	maxMIMEDepth       = 5
)

type Message struct {
	FromName    string
	FromEmail   string
	To          []string
	CC          []string
	Subject     string
	Body        string
	Links       []string
	Attachments []string
	Headers     map[string]string
}

func BuildState(me string, message Message) State {
	links := message.Links
	if len(links) == 0 {
		links = extractLinks(message.Body)
	}
	email := Email{
		From:        formatAddress(message.FromName, message.FromEmail),
		To:          append([]string{}, message.To...),
		CC:          append([]string{}, message.CC...),
		Subject:     message.Subject,
		Body:        truncateRunes(message.Body, maxBodyCharacters),
		Links:       append([]string{}, links...),
		Attachments: append([]string{}, message.Attachments...),
		Headers:     make(map[string]string, len(message.Headers)),
	}
	for key, value := range message.Headers {
		email.Headers[key] = value
	}
	return State{Me: strings.TrimSpace(me), Email: email}
}

func LoadState(ctx context.Context, db *storage.DB, owner string, messageID int64) (State, error) {
	if db == nil {
		return State{}, errors.New("jev loader requires a database")
	}
	snapshot, err := db.SnapshotRawMessage(ctx, owner, messageID)
	if err != nil {
		return State{}, err
	}
	info := snapshot.Info()
	if strings.TrimSpace(info.RawPath) == "" {
		return State{}, errors.New("message has no raw source")
	}
	file, err := os.Open(info.RawPath)
	if err != nil {
		return State{}, err
	}
	defer file.Close()
	message, err := readRawMessage(bufio.NewReader(io.LimitReader(file, maxRawMessageBytes)))
	if err != nil {
		return State{}, err
	}
	if strings.TrimSpace(message.Subject) == "" {
		message.Subject = info.Subject
	}
	message.Links = extractLinks(message.Body)
	return BuildState(info.EmailAddress, message), nil
}

func readRawMessage(reader *bufio.Reader) (Message, error) {
	parsed, err := mail.ReadMessage(reader)
	if err != nil {
		return Message{}, err
	}
	var message Message
	message.FromName, message.FromEmail = parseFirstAddress(parsed.Header.Get("From"))
	message.To = parseAddressList(parsed.Header.Get("To"))
	message.CC = parseAddressList(parsed.Header.Get("Cc"))
	message.Subject = parsed.Header.Get("Subject")
	message.Headers = selectedHeaders(parsed.Header)
	var collector mimeCollector
	if err := walkMIME(textproto.MIMEHeader(parsed.Header), parsed.Body, 0, &collector); err != nil {
		return Message{}, err
	}
	body := collector.plain
	if strings.TrimSpace(body) == "" {
		body = htmlToText(collector.html)
	}
	message.Body = strings.TrimSpace(body)
	message.Attachments = collector.attachments
	return message, nil
}

type mimeCollector struct {
	plain       string
	html        string
	attachments []string
	parts       int
}

func walkMIME(header textproto.MIMEHeader, body io.Reader, depth int, collector *mimeCollector) error {
	if depth > maxMIMEDepth {
		return errors.New("message MIME nesting is too deep")
	}
	collector.parts++
	if collector.parts > maxMIMEParts {
		return errors.New("message has too many MIME parts")
	}
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		mediaType = "text/plain"
	}
	disposition, dispositionParams, _ := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := dispositionParams["filename"]
	if filename == "" {
		filename = params["name"]
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := walkMIME(part.Header, part, depth+1, collector); err != nil {
				part.Close()
				return err
			}
			part.Close()
		}
	}
	if strings.EqualFold(disposition, "attachment") || filename != "" {
		if len(collector.attachments) < maxAttachments {
			collector.attachments = append(collector.attachments, attachmentLabel(filename, mediaType))
		}
		return nil
	}
	switch strings.ToLower(mediaType) {
	case "text/plain":
		if collector.plain == "" {
			collector.plain = readTextPart(header, body)
		}
	case "text/html":
		if collector.html == "" {
			collector.html = readTextPart(header, body)
		}
	}
	return nil
}

func readTextPart(header textproto.MIMEHeader, body io.Reader) string {
	var reader io.Reader = body
	switch strings.ToLower(strings.TrimSpace(header.Get("Content-Transfer-Encoding"))) {
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, reader)
	case "quoted-printable":
		reader = quotedprintable.NewReader(reader)
	}
	payload, err := io.ReadAll(io.LimitReader(reader, 256<<10))
	if err != nil {
		return ""
	}
	return string(payload)
}

func parseFirstAddress(value string) (name, address string) {
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return "", strings.TrimSpace(value)
	}
	return parsed.Name, parsed.Address
}

func parseAddressList(value string) []string {
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		if strings.TrimSpace(value) == "" {
			return []string{}
		}
		return []string{strings.TrimSpace(value)}
	}
	result := make([]string, 0, len(parsed))
	for _, address := range parsed {
		result = append(result, address.String())
	}
	return result
}

func selectedHeaders(header mail.Header) map[string]string {
	keys := []string{
		"Authentication-Results", "List-Unsubscribe", "List-Unsubscribe-Post",
		"Reply-To", "Return-Path", "Precedence", "Auto-Submitted",
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			result[key] = truncateRunes(value, 1000)
		}
	}
	return result
}

func formatAddress(name, address string) string {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if address == "" {
		return name
	}
	if name == "" {
		return address
	}
	return (&mail.Address{Name: name, Address: address}).String()
}

func attachmentLabel(filename, mediaType string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return mediaType
	}
	if mediaType == "" {
		return filename
	}
	return filename + " (" + mediaType + ")"
}

var (
	htmlTagPattern  = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlBreak       = regexp.MustCompile(`(?i)</?(br|p|div|li|tr|h[1-6])[^>]*>`)
	htmlTag         = regexp.MustCompile(`(?s)<[^>]+>`)
	linkPattern     = regexp.MustCompile(`(?i)\bhttps?://[^\s<>"']+`)
	trailingPunct   = regexp.MustCompile(`[),.;:!?]+$`)
	httpHostPattern = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://([^/?#]+)`)
)

func htmlToText(value string) string {
	value = htmlTagPattern.ReplaceAllString(value, " ")
	value = htmlBreak.ReplaceAllString(value, "\n")
	value = htmlTag.ReplaceAllString(value, " ")
	return html.UnescapeString(value)
}

func extractLinks(body string) []string {
	matches := linkPattern.FindAllString(body, -1)
	seen := make(map[string]bool)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		match = trailingPunct.ReplaceAllString(match, "")
		hostMatch := httpHostPattern.FindStringSubmatch(match)
		if len(hostMatch) != 2 {
			continue
		}
		host := strings.ToLower(hostMatch[1])
		if strings.Contains(host, "@") {
			continue
		}
		if at := strings.LastIndex(host, ":"); at > 0 && !strings.Contains(host[at:], "]") {
			host = host[:at]
		}
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		result = append(result, host)
	}
	return result
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
