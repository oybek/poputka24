package telegram

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/oybek/choguuket/database"
	"github.com/oybek/choguuket/model"
	"github.com/samber/lo"
	"github.com/sashabaranov/go-openai"
)

func (lp *LongPoll) handleVoice(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveMessage.Chat
	voice := ctx.EffectiveMessage.Voice

	if voice.Duration > 20 {
		return lp.sendText(chat.Id, TextTooLongVoice)
	}

	text, err := lp.transcribeVoice(voice)
	if err != nil {
		return err
	}

	medicineNames := lo.Map(strings.Split(text, ","), func(s string, _ int) string {
		return strings.TrimSpace(s)
	})

	tuples, err := lp.searchApteka(medicineNames)
	if err != nil {
		return err
	}

	for _, tuple := range tuples {
		text, _ := toMessage(tuple)
		lp.sendText(chat.Id, text)
		time.Sleep(2 * time.Second)
	}

	if len(tuples) == 0 {
		return lp.sendText(
			chat.Id,
			fmt.Sprintf("Не нашел данные лекарства ни в одной из аптек: %s", text),
		)
	}
	return nil
}

func toMessage(t lo.Tuple2[model.Apteka, []string]) (string, *gotgbot.SendMessageOpts) {
	a, ms := t.A, t.B
	baseInfo := fmt.Sprintf("Аптека: %s\nАдрес: %s\nPhone: %s", a.Name, a.Address, a.Phone)
	presenceInfo := fmt.Sprintf("В наличии: %s", strings.Join(ms, ", "))
	return baseInfo + "\n\n" + presenceInfo, &gotgbot.SendMessageOpts{}
}

func (lp *LongPoll) searchApteka(medicineNames []string) ([]lo.Tuple2[model.Apteka, []string], error) {
	return database.Transact(
		lp.db,
		func(tx database.TransactionOps) ([]lo.Tuple2[model.Apteka, []string], error) {
			mIds, err := database.MedicineSearch(tx, medicineNames)
			if err != nil {
				return nil, err
			}

			return database.AptekaSearch(tx, mIds)
		},
	)
}

func (lp *LongPoll) transcribeVoice(voice *gotgbot.Voice) (string, error) {
	file, err := lp.bot.GetFile(voice.FileId, &gotgbot.GetFileOpts{})
	if err != nil {
		return "", err
	}

	resp, err := http.Get(file.URL(lp.bot, &gotgbot.RequestOpts{}))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		Reader:   resp.Body,
		FilePath: file.FilePath,
		Prompt:   "Парацетамол, ТайлолХот, Тримол",
		Language: "ru",
	}

	context := context.Background()
	openaiResp, err := lp.openaiClient.CreateTranscription(context, req)
	if err != nil {
		fmt.Printf("Transcription error: %v\n", err)
		return "", err
	}

	return openaiResp.Text, nil
}
