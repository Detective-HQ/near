"use client";

import "./globals.css";
import "./workspace/workspace.css";

export default function GlobalError() {
  return (
    <html lang="en">
      <body>
        <title>Zonyx</title>
        <main className="ws-fullpage" role="alert">
          <h1>Something broke on our side.</h1>
          <button type="button" className="button" onClick={() => window.location.reload()}>Reload</button>
        </main>
      </body>
    </html>
  );
}
