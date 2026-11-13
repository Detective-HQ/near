"use client";

import "./workspace/workspace.css";

export default function ErrorPage({ retry }: { error: unknown; retry: () => void }) {
  return (
    <main className="ws-fullpage" role="alert">
      <h1>Something broke on our side.</h1>
      <button
        type="button"
        className="button"
        onClick={() => {
          if (typeof retry === "function") retry();
          else window.location.reload();
        }}
      >
        Reload
      </button>
    </main>
  );
}
