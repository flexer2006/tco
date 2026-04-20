package adapters

import (
	"fmt"
	"html"
	"github.com/flexer2006/tco/internal/domain"
)

func authPageHTML(snapshot domain.Snapshot, csrfToken string) string {
	escapedCSRFToken := html.EscapeString(csrfToken)
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Telegram Onboarding</title>
  <style>
    :root {
      --bg: #f2f6fb;
      --card: #ffffff;
      --text: #1f2a37;
      --muted: #5b6778;
      --primary: #0f766e;
      --primary-hover: #0b5f59;
      --line: #d7deea;
      --field: #f8fbff;
      --danger: #b91c1c;
      --radius: 14px;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Manrope", "Segoe UI", "Noto Sans", sans-serif;
      color: var(--text);
      background:
        radial-gradient(1200px 600px at 85%% -10%%, #d5f5f1 0%%, transparent 55%%),
        radial-gradient(900px 500px at -20%% 110%%, #dbeafe 0%%, transparent 55%%),
        var(--bg);
      display: grid;
      place-items: center;
      padding: 20px;
    }
    .panel {
      width: min(860px, 100%%);
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: var(--radius);
      box-shadow: 0 18px 60px rgba(20, 35, 60, 0.08);
      overflow: hidden;
    }
    .head {
      padding: 24px 24px 16px;
      border-bottom: 1px solid var(--line);
      text-align: center;
    }
    h1 {
      margin: 0;
      font-size: 30px;
      line-height: 1.15;
      letter-spacing: -0.02em;
    }
    .subtitle {
      margin: 10px auto 0;
      color: var(--muted);
      max-width: 620px;
      font-size: 14px;
      line-height: 1.45;
    }
    .meta {
      display: flex;
      flex-wrap: wrap;
      justify-content: center;
      gap: 8px;
      margin-top: 14px;
    }
    .chip {
      border: 1px solid var(--line);
      background: #f9fbff;
      border-radius: 999px;
      padding: 6px 12px;
      font-size: 12px;
      color: #334155;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 14px;
      padding: 18px;
    }
    .card {
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      background: #ffffff;
    }
    .card h2 {
      margin: 0 0 6px;
      font-size: 18px;
      line-height: 1.25;
    }
    .hint {
      margin: 0 0 12px;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.45;
    }
    label {
      display: block;
      margin: 10px 0 6px;
      font-size: 13px;
      font-weight: 600;
    }
    input {
      width: 100%%;
      border: 1px solid var(--line);
      border-radius: 10px;
      background: var(--field);
      padding: 10px 12px;
      font-size: 14px;
      color: var(--text);
      outline: none;
    }
    input:focus {
      border-color: var(--primary);
      box-shadow: 0 0 0 3px rgba(15, 118, 110, 0.15);
      background: #fff;
    }
    button {
      width: 100%%;
      margin-top: 12px;
      border: 0;
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 14px;
      font-weight: 700;
      color: #fff;
      background: var(--primary);
      cursor: pointer;
    }
    button:hover { background: var(--primary-hover); }
    .logout button {
      background: var(--danger);
    }
    .logout button:hover {
      background: #991b1b;
    }
    @media (max-width: 760px) {
      .grid { grid-template-columns: 1fr; }
      .head { text-align: left; }
      .meta { justify-content: flex-start; }
    }
  </style>
</head>
<body>
  <main class="panel">
    <header class="head">
      <h1>Telegram onboarding</h1>
      <p class="subtitle">
        Complete steps from left to right: start login, enter code from Telegram, and enter cloud password only if Telegram asks for 2FA.
      </p>
      <div class="meta">
        <span class="chip">State: %s</span>
        <span class="chip">Phone: %s</span>
      </div>
    </header>
    <section class="grid">
      <form class="card" method="post" action="/auth/start">
        <h2>1. Start login</h2>
        <p class="hint">Use your Telegram API credentials and phone in international format.</p>
        <input type="hidden" name="%s" value="%s">
        <label for="api-id">API ID</label>
        <input id="api-id" name="api_id" autocomplete="off">
        <label for="api-hash">API Hash</label>
        <input id="api-hash" name="api_hash" autocomplete="off">
        <label for="phone">Phone</label>
        <input id="phone" name="phone" placeholder="+1234567890" autocomplete="tel">
        <button type="submit">Start</button>
      </form>

      <form class="card" method="post" action="/auth/verify-code">
        <h2>2. Verify code</h2>
        <p class="hint">Enter the login code you received in Telegram.</p>
        <input type="hidden" name="%s" value="%s">
        <label for="code">Code</label>
        <input id="code" name="code" autocomplete="one-time-code">
        <button type="submit">Verify code</button>
      </form>

      <form class="card" method="post" action="/auth/verify-password">
        <h2>3. Verify 2FA password</h2>
        <p class="hint">Only needed if your Telegram account has cloud password enabled.</p>
        <input type="hidden" name="%s" value="%s">
        <label for="password">Password</label>
        <input id="password" type="password" name="password" autocomplete="current-password">
        <button type="submit">Verify password</button>
      </form>

      <form class="card logout" method="post" action="/auth/logout">
        <h2>Session</h2>
        <p class="hint">Reset saved session and return onboarding to initial state.</p>
        <input type="hidden" name="%s" value="%s">
        <button type="submit">Logout</button>
      </form>
    </section>
  </main>
</body>
</html>
`,
		html.EscapeString(string(snapshot.State)),
		html.EscapeString(snapshot.Phone),
		csrfFormField, escapedCSRFToken,
		csrfFormField, escapedCSRFToken,
		csrfFormField, escapedCSRFToken,
		csrfFormField, escapedCSRFToken,
	)
}
