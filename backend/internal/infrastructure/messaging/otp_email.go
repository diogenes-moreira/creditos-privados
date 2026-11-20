package messaging

import (
	"bytes"
	"fmt"
	"html/template"
)

// DefaultEmailLogoURL is the CuppoGan wordmark the app already serves
// publicly. Mail clients fetch it from there, so the email carries no
// attachment.
const DefaultEmailLogoURL = "https://app.cuppogan.com/logo.png"

// otpEmail is the content of a one-time-code email in both formats SendGrid
// sends: plain text for clients that do not render HTML, and the branded HTML.
type otpEmail struct {
	Subject string
	Text    string
	HTML    string
}

// otpEmailData fills the HTML template.
type otpEmailData struct {
	Code    string
	LogoURL string
}

// Brand colours from the CuppoGan brandbook v2.0 (frontend/src/theme.ts).
// Email clients ignore CSS variables and most <style> blocks, so every style
// is inline and the gradient is replaced by flat Deep Field green.
var otpEmailTemplate = template.Must(template.New("otp").Parse(`<!DOCTYPE html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light">
<title>Tu código de acceso a CuppoGan</title>
</head>
<body style="margin:0;padding:0;background-color:#F4F2EB;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">Tu código es {{.Code}}. Vence en 10 minutos.</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="background-color:#F4F2EB;">
  <tr>
    <td align="center" style="padding:32px 16px;">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:480px;background-color:#FFFFFF;border-radius:12px;overflow:hidden;">
        <tr>
          <td style="height:6px;line-height:6px;font-size:0;background-color:#06673D;">&nbsp;</td>
        </tr>
        <tr>
          <td align="center" style="padding:32px 32px 8px 32px;">
            <img src="{{.LogoURL}}" width="180" alt="CuppoGan" style="display:block;width:180px;max-width:100%;height:auto;border:0;">
          </td>
        </tr>
        <tr>
          <td style="padding:24px 32px 0 32px;font-family:Inter,Arial,sans-serif;color:#2D3131;">
            <h1 style="margin:0 0 12px 0;font-size:20px;line-height:28px;font-weight:700;color:#2D3131;">Tu código de acceso</h1>
            <p style="margin:0;font-size:15px;line-height:22px;color:#5C6160;">Escribe este código en CuppoGan para ingresar a tu cuenta.</p>
          </td>
        </tr>
        <tr>
          <td align="center" style="padding:24px 32px;">
            <table role="presentation" cellpadding="0" cellspacing="0" border="0" style="background-color:#F4F2EB;border-radius:8px;">
              <tr>
                <td style="padding:16px 28px;font-family:'Courier New',Courier,monospace;font-size:34px;line-height:40px;font-weight:700;letter-spacing:8px;color:#06673D;">{{.Code}}</td>
              </tr>
            </table>
          </td>
        </tr>
        <tr>
          <td style="padding:0 32px 32px 32px;font-family:Inter,Arial,sans-serif;font-size:14px;line-height:21px;color:#5C6160;">
            <p style="margin:0 0 8px 0;">Vence en <strong style="color:#2D3131;">10 minutos</strong> y se usa una sola vez.</p>
            <p style="margin:0;">Si no pediste este código, ignora este mensaje: nadie podrá entrar a tu cuenta sin él.</p>
          </td>
        </tr>
        <tr>
          <td style="padding:16px 32px;border-top:1px solid #E6E3D9;font-family:Inter,Arial,sans-serif;font-size:12px;line-height:18px;color:#8A8F8E;">
            CuppoGan nunca te pedirá este código por teléfono ni por mensaje. No lo compartas con nadie.
          </td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>
`))

// buildOTPEmail renders the one-time-code email.
func buildOTPEmail(code, logoURL string) (otpEmail, error) {
	if logoURL == "" {
		logoURL = DefaultEmailLogoURL
	}
	var html bytes.Buffer
	if err := otpEmailTemplate.Execute(&html, otpEmailData{Code: code, LogoURL: logoURL}); err != nil {
		return otpEmail{}, fmt.Errorf("rendering the email: %w", err)
	}
	return otpEmail{
		// Mail clients truncate subjects, so the code goes first.
		Subject: fmt.Sprintf("%s es tu código de acceso a CuppoGan", code),
		Text: fmt.Sprintf(
			"Tu código de acceso a CuppoGan es %s.\n\n"+
				"Vence en 10 minutos y se usa una sola vez.\n"+
				"Si no pediste este código, ignora este mensaje.\n\n"+
				"CuppoGan nunca te pedirá este código por teléfono ni por mensaje.",
			code),
		HTML: html.String(),
	}, nil
}
