package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beevik/etree"
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
)

const cbrEndpoint = "https://www.cbr.ru/DailyInfoWebServ/DailyInfo.asmx"

// CBRService получает ключевую ставку ЦБ РФ через SOAP и кеширует её.
type CBRService struct {
	client       *http.Client
	log          *logrus.Logger
	margin       float64
	fallbackRate float64

	mu         sync.RWMutex
	cachedRate float64
	cachedAt   time.Time
	cacheTTL   time.Duration
}

// NewCBRService создаёт клиент интеграции с ЦБ РФ.
func NewCBRService(cfg *config.Config, log *logrus.Logger) *CBRService {
	return &CBRService{
		client:       &http.Client{Timeout: 10 * time.Second},
		log:          log,
		margin:       cfg.BankMargin,
		fallbackRate: cfg.FallbackRate,
		cacheTTL:     time.Hour,
	}
}

// buildSOAPRequest формирует SOAP-конверт запроса KeyRate за последние 30 дней.
func buildSOAPRequest() string {
	fromDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	toDate := time.Now().Format("2006-01-02")
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<soap12:Envelope xmlns:soap12="http://www.w3.org/2003/05/soap-envelope">
  <soap12:Body>
    <KeyRate xmlns="http://web.cbr.ru/">
      <fromDate>%s</fromDate>
      <ToDate>%s</ToDate>
    </KeyRate>
  </soap12:Body>
</soap12:Envelope>`, fromDate, toDate)
}

func (s *CBRService) sendRequest(ctx context.Context, soapRequest string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cbrEndpoint,
		bytes.NewBufferString(soapRequest))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", "http://web.cbr.ru/KeyRate")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к ЦБ РФ: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа ЦБ РФ: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ЦБ РФ вернул статус %d", resp.StatusCode)
	}
	return body, nil
}

// parseXMLResponse извлекает последнее значение ключевой ставки из ответа.
func parseXMLResponse(raw []byte) (float64, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(raw); err != nil {
		return 0, fmt.Errorf("ошибка парсинга XML: %w", err)
	}
	elements := doc.FindElements("//diffgram/KeyRate/KR")
	if len(elements) == 0 {
		return 0, errors.New("данные по ключевой ставке не найдены")
	}
	// Первый элемент содержит самое свежее значение.
	rateElement := elements[0].FindElement("./Rate")
	if rateElement == nil {
		return 0, errors.New("тег Rate отсутствует в ответе")
	}
	rateStr := strings.TrimSpace(rateElement.Text())
	rate, err := strconv.ParseFloat(strings.ReplaceAll(rateStr, ",", "."), 64)
	if err != nil {
		return 0, fmt.Errorf("ошибка конвертации ставки %q: %w", rateStr, err)
	}
	return rate, nil
}

// KeyRate возвращает ключевую ставку ЦБ РФ (с кешированием на час).
func (s *CBRService) KeyRate(ctx context.Context) (float64, error) {
	s.mu.RLock()
	if s.cachedRate > 0 && time.Since(s.cachedAt) < s.cacheTTL {
		rate := s.cachedRate
		s.mu.RUnlock()
		return rate, nil
	}
	s.mu.RUnlock()

	raw, err := s.sendRequest(ctx, buildSOAPRequest())
	if err != nil {
		return 0, err
	}
	rate, err := parseXMLResponse(raw)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	s.cachedRate, s.cachedAt = rate, time.Now()
	s.mu.Unlock()

	return rate, nil
}

// CreditRate возвращает ставку кредитования: ключевая ставка + маржа банка.
// Если ЦБ РФ недоступен, используется резервное значение из конфигурации.
func (s *CBRService) CreditRate(ctx context.Context) float64 {
	rate, err := s.KeyRate(ctx)
	if err != nil {
		s.log.Warnf("не удалось получить ставку ЦБ РФ, используется резервная %.2f%%: %v",
			s.fallbackRate, err)
		rate = s.fallbackRate
	}
	return round2(rate + s.margin)
}
