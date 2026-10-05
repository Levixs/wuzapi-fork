package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// parseNewsletterJID builds a newsletter JID from a bare id (no @) or parses a full jid.
func parseNewsletterJID(arg string) (types.JID, bool) {
	if arg == "" {
		return types.JID{}, false
	}
	if len(arg) > 0 && arg[0] != '+' && !containsAt(arg) {
		return types.NewJID(arg, types.NewsletterServer), true
	}
	return parseJID(arg)
}

func containsAt(s string) bool {
	for _, c := range s {
		if c == '@' {
			return true
		}
	}
	return false
}

// CreateNewsletter creates a new WhatsApp channel (newsletter).
func (s *server) CreateNewsletter() http.HandlerFunc {

	type createNewsletterStruct struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Picture     string `json:"picture"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t createNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		if t.Name == "" {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Name in Payload"))
			return
		}

		var picture []byte
		if t.Picture != "" {
			if len(t.Picture) > 10 && t.Picture[0:10] == "data:image" {
				dataURL, err := dataurl.DecodeString(t.Picture)
				if err != nil {
					s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode base64 encoded Picture from payload"))
					return
				}
				picture = dataURL.Data
			}
		}

		// Aceita o ToS de criação de newsletter antes do create — o fork não faz isso automaticamente
		// (diferente do Baileys/zapo-js, que auto-aceitam). Falha silenciosa se já aceito antes.
		_ = client.AcceptTOSNotice(r.Context(), "20601218", "5")

		info, err := client.CreateNewsletter(r.Context(), whatsmeow.CreateNewsletterParams{
			Name:        t.Name,
			Description: t.Description,
			Picture:     picture,
		})

		if err != nil {
			msg := fmt.Sprintf("failed to create newsletter: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(info)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// GetNewsletterInfo fetches a newsletter's metadata by JID or invite code.
func (s *server) GetNewsletterInfo() http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		id := r.URL.Query().Get("id")
		invite := r.URL.Query().Get("invite")

		var info *types.NewsletterMetadata
		var err error

		if invite != "" {
			info, err = client.GetNewsletterInfoWithInvite(r.Context(), invite)
		} else if id != "" {
			jid, ok := parseNewsletterJID(id)
			if !ok {
				s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
				return
			}
			info, err = client.GetNewsletterInfo(r.Context(), jid)
		} else {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing id or invite parameter"))
			return
		}

		if err != nil {
			msg := fmt.Sprintf("failed to get newsletter info: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		responseJson, err := json.Marshal(info)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}

		return
	}
}

// FollowNewsletter subscribes the current account to a newsletter.
func (s *server) FollowNewsletter() http.HandlerFunc {
	return s.newsletterFollowHandler(true)
}

// UnfollowNewsletter unsubscribes the current account from a newsletter.
func (s *server) UnfollowNewsletter() http.HandlerFunc {
	return s.newsletterFollowHandler(false)
}

func (s *server) newsletterFollowHandler(follow bool) http.HandlerFunc {

	type followNewsletterStruct struct {
		Id string `json:"id"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t followNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		if follow {
			err = client.FollowNewsletter(context.Background(), jid)
		} else {
			err = client.UnfollowNewsletter(context.Background(), jid)
		}

		if err != nil {
			msg := fmt.Sprintf("failed to update newsletter subscription: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"ok": true})
		return
	}
}

// MuteNewsletter mutes or unmutes a newsletter for the current account.
func (s *server) MuteNewsletter() http.HandlerFunc {

	type muteNewsletterStruct struct {
		Id   string `json:"id"`
		Mute bool   `json:"mute"`
	}

	return func(w http.ResponseWriter, r *http.Request) {

		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t muteNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		err = client.NewsletterToggleMute(context.Background(), jid, t.Mute)
		if err != nil {
			msg := fmt.Sprintf("failed to toggle newsletter mute: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, msg)
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"ok": true})
		return
	}
}

// Publishes to a newsletter/channel: text, an image with caption, or text with a custom link
// preview card. Só o dono pode enviar (limitação do WhatsApp).
func (s *server) SendNewsletterMessage() http.HandlerFunc {

	type sendNewsletterStruct struct {
		Id                  string
		Text                string
		Image               string // base64 (JPEG or PNG), with or without data URI prefix
		Caption             string // caption for Image and Video
		Video               string // base64 MP4
		Audio               string // base64 OGG/Opus (sent as a voice note)
		Sticker             string // base64 WebP
		Poll                *newsletterPoll
		LinkPreviewOverride *linkPreviewOverride `json:"LinkPreviewOverride,omitempty"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t sendNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		if t.Text == "" && t.Image == "" && t.Video == "" && t.Audio == "" && t.Sticker == "" && t.Poll == nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing Text, Image, Video, Audio, Sticker or Poll in Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		msg := &waE2E.Message{Conversation: proto.String(t.Text)}
		var extra whatsmeow.SendRequestExtra
		var buildErr error
		switch {
		case t.Image != "":
			var imageMsg *waE2E.ImageMessage
			imageMsg, extra.MediaHandle, buildErr = buildNewsletterImage(r.Context(), client, t.Image, t.Caption)
			msg = &waE2E.Message{ImageMessage: imageMsg}
		case t.Video != "":
			var up whatsmeow.UploadResponse
			if up, buildErr = uploadNewsletterBase64(r.Context(), client, t.Video, whatsmeow.MediaVideo); buildErr == nil {
				extra.MediaHandle = up.Handle
				videoMsg := &waE2E.VideoMessage{
					URL:        proto.String(up.URL),
					DirectPath: proto.String(up.DirectPath),
					FileSHA256: up.FileSHA256,
					FileLength: proto.Uint64(up.FileLength),
					Mimetype:   proto.String("video/mp4"),
				}
				if t.Caption != "" {
					videoMsg.Caption = proto.String(t.Caption)
				}
				msg = &waE2E.Message{VideoMessage: videoMsg}
			}
		case t.Audio != "":
			var up whatsmeow.UploadResponse
			if up, buildErr = uploadNewsletterBase64(r.Context(), client, t.Audio, whatsmeow.MediaAudio); buildErr == nil {
				extra.MediaHandle = up.Handle
				msg = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
					URL:        proto.String(up.URL),
					DirectPath: proto.String(up.DirectPath),
					FileSHA256: up.FileSHA256,
					FileLength: proto.Uint64(up.FileLength),
					Mimetype:   proto.String("audio/ogg; codecs=opus"),
					PTT:        proto.Bool(true),
				}}
			}
		case t.Sticker != "":
			var up whatsmeow.UploadResponse
			if up, buildErr = uploadNewsletterBase64(r.Context(), client, t.Sticker, whatsmeow.MediaImage); buildErr == nil {
				extra.MediaHandle = up.Handle
				msg = &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
					URL:        proto.String(up.URL),
					DirectPath: proto.String(up.DirectPath),
					FileSHA256: up.FileSHA256,
					FileLength: proto.Uint64(up.FileLength),
					Mimetype:   proto.String("image/webp"),
				}}
			}
		case t.Poll != nil:
			if len(t.Poll.Options) < 2 || len(t.Poll.Options) > 12 || t.Poll.Name == "" {
				buildErr = errors.New("Poll needs a Name and 2 to 12 Options")
			} else {
				selectable := t.Poll.SelectableCount
				if selectable < 1 {
					selectable = 1
				}
				msg = client.BuildPollCreation(t.Poll.Name, t.Poll.Options, selectable)
			}
		case t.LinkPreviewOverride != nil:
			msg = buildNewsletterLinkPreview(r.Context(), client, t.Text, t.LinkPreviewOverride)
		}
		if buildErr != nil {
			s.Respond(w, r, http.StatusBadRequest, buildErr)
			return
		}

		resp, err := client.SendMessage(r.Context(), jid, msg, extra)
		if err != nil {
			msg := fmt.Sprintf("failed to send newsletter message: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		response := map[string]interface{}{"Id": resp.ID, "ServerId": resp.ServerID}
		responseJson, err := json.Marshal(response)
		if err != nil {
			s.Respond(w, r, http.StatusInternalServerError, err)
		} else {
			s.Respond(w, r, http.StatusOK, string(responseJson))
		}
	}
}

type newsletterPoll struct {
	Name            string
	Options         []string
	SelectableCount int
}

// uploadNewsletterBase64 decodes a base64 payload and uploads it unencrypted (channels have no media key).
func uploadNewsletterBase64(ctx context.Context, client *whatsmeow.Client, raw string, mediaType whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
	data, err := decodeBase64Payload(raw)
	if err != nil {
		return whatsmeow.UploadResponse{}, errors.New("invalid base64 in media payload")
	}
	uploaded, err := client.UploadNewsletter(ctx, data, mediaType)
	if err != nil {
		return whatsmeow.UploadResponse{}, fmt.Errorf("failed to upload newsletter media: %w", err)
	}
	return uploaded, nil
}

// buildNewsletterImage uploads the image unencrypted (channels have no media key) and returns the
// message plus the upload handle that must accompany the send request.
func buildNewsletterImage(ctx context.Context, client *whatsmeow.Client, rawImage, caption string) (*waE2E.ImageMessage, string, error) {
	data, err := decodeBase64Payload(rawImage)
	if err != nil {
		return nil, "", errors.New("invalid base64 in Image")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, "", errors.New("Image must be JPEG or PNG")
	}
	uploaded, err := client.UploadNewsletter(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, "", fmt.Errorf("failed to upload newsletter image: %w", err)
	}
	imageMsg := &waE2E.ImageMessage{
		URL:        proto.String(uploaded.URL),
		DirectPath: proto.String(uploaded.DirectPath),
		FileSHA256: uploaded.FileSHA256,
		FileLength: proto.Uint64(uploaded.FileLength),
		Mimetype:   proto.String("image/" + format),
		Width:      proto.Uint32(uint32(cfg.Width)),
		Height:     proto.Uint32(uint32(cfg.Height)),
	}
	if caption != "" {
		imageMsg.Caption = proto.String(caption)
	}
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		if thumb, err := jpegThumbnail(img, 72, 72); err == nil {
			imageMsg.JPEGThumbnail = thumb
		}
	}
	return imageMsg, uploaded.Handle, nil
}

// buildNewsletterLinkPreview builds the card with an unencrypted HQ thumbnail; if that upload
// fails the card still goes out with the inline thumbnail only.
func buildNewsletterLinkPreview(ctx context.Context, client *whatsmeow.Client, body string, o *linkPreviewOverride) *waE2E.Message {
	url, og := buildLinkPreviewOverride(body, o)
	etm := &waE2E.ExtendedTextMessage{
		Text:          proto.String(body),
		MatchedText:   proto.String(url),
		Title:         proto.String(og.Title),
		Description:   proto.String(og.Description),
		JPEGThumbnail: og.ImageData,
	}
	if len(og.HQImageData) > 0 {
		uploaded, err := client.UploadNewsletter(ctx, og.HQImageData, whatsmeow.MediaLinkThumbnail)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to upload newsletter link preview thumbnail, sending inline thumbnail only")
		} else {
			etm.ThumbnailDirectPath = proto.String(uploaded.DirectPath)
			etm.ThumbnailSHA256 = uploaded.FileSHA256
			etm.ThumbnailWidth = proto.Uint32(og.HQWidth)
			etm.ThumbnailHeight = proto.Uint32(og.HQHeight)
		}
	}
	return &waE2E.Message{ExtendedTextMessage: etm}
}

// React to a specific message inside a newsletter/channel.
//
// NAO TESTADO contra WhatsApp real -- so compilado (ver .ai/context do wpp-api, achado 2026-09).
func (s *server) SendNewsletterReaction() http.HandlerFunc {

	type reactNewsletterStruct struct {
		Id       string // newsletter id
		ServerId int    // target message's server id (types.MessageServerID)
		Reaction string // emoji, or "" to remove
	}

	return func(w http.ResponseWriter, r *http.Request) {
		txtid := r.Context().Value("userinfo").(Values).Get("Id")
		client := clientManager.GetWhatsmeowClient(txtid)
		if client == nil {
			s.Respond(w, r, http.StatusInternalServerError, errors.New("no session"))
			return
		}

		decoder := json.NewDecoder(r.Body)
		var t reactNewsletterStruct
		err := decoder.Decode(&t)
		if err != nil {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not decode Payload"))
			return
		}
		if t.ServerId == 0 {
			s.Respond(w, r, http.StatusBadRequest, errors.New("missing ServerId in Payload"))
			return
		}

		jid, ok := parseNewsletterJID(t.Id)
		if !ok {
			s.Respond(w, r, http.StatusBadRequest, errors.New("could not parse newsletter id"))
			return
		}

		err = client.NewsletterSendReaction(context.Background(), jid, types.MessageServerID(t.ServerId), t.Reaction, "")
		if err != nil {
			msg := fmt.Sprintf("failed to react to newsletter message: %v", err)
			log.Error().Msg(msg)
			s.Respond(w, r, http.StatusInternalServerError, errors.New(msg))
			return
		}

		s.Respond(w, r, http.StatusOK, map[string]bool{"ok": true})
	}
}
