package service

import (
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
	mail "gopkg.in/mail.v2"

	"bankapi/internal/config"
)

// Mailer отправляет уведомления пользователям по SMTP.
type Mailer struct {
	cfg config.SMTPConfig
	log *logrus.Logger
}

// NewMailer создаёт сервис почтовых уведомлений.
func NewMailer(cfg *config.Config, log *logrus.Logger) *Mailer {
	return &Mailer{cfg: cfg.SMTP, log: log}
}

func (m *Mailer) dialer() *mail.Dialer {
	d := mail.NewDialer(m.cfg.Host, m.cfg.Port, m.cfg.User, m.cfg.Password)
	d.TLSConfig = &tls.Config{
		ServerName:         m.cfg.Host,
		InsecureSkipVerify: false,
	}
	// Локальный dev-сервер (MailHog) работает без TLS и аутентификации:
	// STARTTLS применяется оппортунистически, только если сервер его анонсирует.
	return d
}

func (m *Mailer) message(to, subject, body string) *mail.Message {
	msg := mail.NewMessage()
	msg.SetHeader("From", m.cfg.From)
	msg.SetHeader("To", to)
	msg.SetHeader("Subject", subject)
	msg.SetBody("text/html", body)
	return msg
}

// Send отправляет письмо синхронно.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.cfg.Enabled {
		m.log.Debugf("SMTP отключён, письмо для %s не отправлено", to)
		return nil
	}
	if err := m.dialer().DialAndSend(m.message(to, subject, body)); err != nil {
		m.log.Errorf("SMTP error: %v", err)
		return fmt.Errorf("не удалось отправить письмо")
	}
	m.log.Infof("письмо отправлено: %s (%s)", to, subject)
	return nil
}

// SendAsync отправляет письмо в фоне: отказ почтового сервиса
// не должен прерывать основную банковскую операцию.
func (m *Mailer) SendAsync(to, subject, body string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.log.Errorf("паника при отправке письма: %v", r)
			}
		}()
		_ = m.Send(to, subject, body)
	}()
}

// --- Шаблоны писем ---

// NotifyWelcome - приветственное письмо после регистрации.
func (m *Mailer) NotifyWelcome(to, username string) {
	body := fmt.Sprintf(`
		<h2>Добро пожаловать, %s!</h2>
		<p>Ваш аккаунт в банковском сервисе успешно создан.</p>
		<small>Это автоматическое уведомление, отвечать на него не нужно.</small>`, username)
	m.SendAsync(to, "Регистрация завершена", body)
}

// NotifyPayment - уведомление об успешной операции.
func (m *Mailer) NotifyPayment(to, title string, amount float64, details string) {
	body := fmt.Sprintf(`
		<h2>%s</h2>
		<p>Сумма: <strong>%.2f RUB</strong></p>
		<p>%s</p>
		<small>Это автоматическое уведомление, отвечать на него не нужно.</small>`,
		title, amount, details)
	m.SendAsync(to, title, body)
}

// NotifyOverdue - уведомление о просрочке и начислении штрафа.
func (m *Mailer) NotifyOverdue(to string, creditID int64, amount, penalty float64) {
	body := fmt.Sprintf(`
		<h2>Просрочен платёж по кредиту №%d</h2>
		<p>Сумма платежа: <strong>%.2f RUB</strong></p>
		<p>Начислен штраф: <strong>%.2f RUB</strong></p>
		<p>Пожалуйста, пополните счёт: списание будет повторено автоматически.</p>
		<small>Это автоматическое уведомление, отвечать на него не нужно.</small>`,
		creditID, amount, penalty)
	m.SendAsync(to, "Просрочка платежа по кредиту", body)
}
