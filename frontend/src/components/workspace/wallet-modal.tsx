"use client";

import { useEffect, useRef, useState } from "react";
import { explorerAddressUrl } from "../../lib/explorer";
import { SUPPORTED_FAMILIES } from "../../lib/wallets";
import type { ChainFamily } from "../../lib/wallets";
import { useWallet } from "../../lib/wallets/context";
import { Modal } from "./ui";
import { useWorkspace } from "./workspace-context";

export function shortenAddress(address: string): string {
  if (!address || address.length <= 12) return address || "Address unavailable";
  return `${address.slice(0, 6)}…${address.slice(-4)}`;
}

interface WalletModalProps {
  open: boolean;
  onClose: () => void;
}

export function WalletModal({ open, onClose }: WalletModalProps) {
  const { state, error, connect, isConnected } = useWallet();
  const { wallet: wsWallet } = useWorkspace();
  const [selectedFamily, setSelectedFamily] = useState<ChainFamily | null>(null);
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    if ((isConnected || wsWallet.connected) && open) {
      onClose();
    }
  }, [isConnected, wsWallet.connected, open, onClose]);

  useEffect(() => {
    if (!open) {
      setSelectedFamily(null);
      setLocalError(null);
    }
  }, [open]);

  const handleSelectFamily = async (family: ChainFamily, label: string) => {
    setSelectedFamily(family);
    setLocalError(null);

    try {
      if (family === "evm") {
        // Connect via multi-chain wallet
        await connect("evm", label);
        // Also sync with workspace EVM wallet
        const opt = wsWallet.options.find((o) => o.family === "evm" || !o.family) ?? wsWallet.options[0];
        if (opt) {
          void wsWallet.connect(opt);
        } else if (typeof window !== "undefined") {
          const eth = (window as unknown as { ethereum?: import("../../lib/evm-wallet").Eip1193Provider }).ethereum;
          if (eth) {
            void wsWallet.connect({ id: "injected", name: label, family: "evm", provider: eth });
          }
        }
      } else {
        await connect(family, label);
      }
      onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to connect wallet.";
      setLocalError(msg);
    }
  };

  const isPending = state === "connecting" || state === "confirming" || wsWallet.connecting;

  return (
    <Modal open={open} title="Choose a wallet" onClose={onClose}>
      {isPending ? (
        <div className="ws-modal-loading" role="status" style={{ padding: "24px 0", textAlign: "center" }}>
          <p className="ws-modal-note" style={{ fontSize: "16px", marginBottom: "8px" }}>
            {state === "confirming" || wsWallet.connecting ? "Confirm in your wallet..." : "Connecting..."}
          </p>
          <p className="subtle-meta">
            Please approve the connection request in your browser extension.
          </p>
        </div>
      ) : (
        <div className="ws-wallet-chooser">
          {(error || localError) && (
            <div
              className="fixture-banner ws-banner"
              role="alert"
              style={{ marginBottom: "16px", borderColor: "rgba(220, 38, 38, 0.4)" }}
            >
              <p>{localError || error}</p>
              {selectedFamily && (
                <button
                  type="button"
                  className="button button-secondary button-small"
                  style={{ marginTop: "8px" }}
                  onClick={() => {
                    const fam = SUPPORTED_FAMILIES.find((f) => f.family === selectedFamily);
                    if (fam) void handleSelectFamily(fam.family, fam.label);
                  }}
                >
                  Retry
                </button>
              )}
            </div>
          )}

          <p className="subtle-meta" style={{ marginBottom: "16px" }}>
            Select an ecosystem to connect your non-custodial wallet. All signing remains client-side.
          </p>

          <ul className="ws-wallet-list" style={{ listStyle: "none", padding: 0, margin: 0 }}>
            {SUPPORTED_FAMILIES.map((fam) => (
              <li key={fam.family} style={{ marginBottom: "10px" }}>
                <button
                  type="button"
                  className="button button-secondary"
                  style={{
                    width: "100%",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    padding: "12px 16px",
                    textAlign: "left",
                  }}
                  onClick={() => void handleSelectFamily(fam.family, fam.label)}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: "12px" }}>
                    <span style={{ fontSize: "20px", width: "24px", textAlign: "center" }} aria-hidden="true">
                      {fam.icon}
                    </span>
                    <div>
                      <div style={{ fontWeight: 600 }}>{fam.label}</div>
                      <div className="subtle-meta" style={{ fontSize: "12px" }}>
                        {fam.description}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`state-pill ${fam.status === "active" ? "good" : ""}`}
                    style={{ fontSize: "11px", textTransform: "capitalize" }}
                  >
                    {fam.status}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Modal>
  );
}

export function MultiChainWalletButton() {
  const { account, isConnected, state, disconnect } = useWallet();
  const { wallet: wsWallet, chains, toast } = useWorkspace();
  const [modalOpen, setModalOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const close = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [menuOpen]);

  const activeAddress = account?.address ?? wsWallet.state.account;
  const activeFamily = account?.chainFamily ?? "evm";
  const activeChainId = account?.chainId ?? wsWallet.state.chainId;
  const isWalletConnected = Boolean(activeAddress) && (isConnected || wsWallet.connected);

  if (!isWalletConnected || !activeAddress) {
    const isBusy = state === "connecting" || state === "confirming" || wsWallet.connecting;
    return (
      <>
        <button
          type="button"
          className="button"
          disabled={isBusy}
          onClick={() => setModalOpen(true)}
        >
          {isBusy ? "Confirm in your wallet..." : "Connect wallet"}
        </button>
        <WalletModal open={modalOpen || wsWallet.pickerOpen} onClose={() => {
          setModalOpen(false);
          wsWallet.closePicker();
        }} />
      </>
    );
  }

  // Connected state: find explorer link
  const chain = chains.data?.find((c) => c.chainId === activeChainId || c.family === activeFamily) ?? null;
  const explorerUrl = explorerAddressUrl(chain, activeAddress);

  return (
    <div className="ws-wallet" ref={menuRef} style={{ position: "relative" }}>
      <button
        type="button"
        className="button button-secondary"
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        onClick={() => setMenuOpen((v) => !v)}
        style={{ display: "flex", alignItems: "center", gap: "8px" }}
      >
        <span className="state-pill good" style={{ padding: "2px 6px", fontSize: "10px" }}>
          {activeFamily.toUpperCase()}
        </span>
        <span>{shortenAddress(activeAddress)}</span>
      </button>

      {menuOpen && (
        <div
          className="ws-menu panel"
          role="menu"
          style={{
            position: "absolute",
            right: 0,
            top: "calc(100% + 6px)",
            minWidth: "190px",
            zIndex: 50,
          }}
        >
          <div style={{ padding: "8px 12px", borderBottom: "1px solid rgba(255, 255, 255, 0.08)", fontSize: "12px" }}>
            <span className="subtle-meta">Connected via {account?.label ?? activeFamily}</span>
          </div>

          <button
            type="button"
            role="menuitem"
            style={{ width: "100%", textAlign: "left", padding: "8px 12px", background: "none", border: 0, cursor: "pointer", color: "inherit" }}
            onClick={() => {
              void navigator.clipboard?.writeText(activeAddress).then(
                () => toast("Copied"),
                () => undefined
              );
              setMenuOpen(false);
            }}
          >
            Copy address
          </button>

          {explorerUrl ? (
            <a
              role="menuitem"
              href={explorerUrl}
              target="_blank"
              rel="noreferrer"
              style={{ display: "block", padding: "8px 12px", textDecoration: "none", color: "inherit" }}
              onClick={() => setMenuOpen(false)}
            >
              View on explorer
            </a>
          ) : (
            <button
              type="button"
              role="menuitem"
              disabled
              style={{ width: "100%", textAlign: "left", padding: "8px 12px", background: "none", border: 0, opacity: 0.5, cursor: "not-allowed", color: "inherit" }}
            >
              View on explorer
            </button>
          )}

          <button
            type="button"
            role="menuitem"
            style={{ width: "100%", textAlign: "left", padding: "8px 12px", background: "none", border: 0, cursor: "pointer", color: "rgba(239, 68, 68, 0.9)", borderTop: "1px solid rgba(255, 255, 255, 0.08)" }}
            onClick={() => {
              setMenuOpen(false);
              wsWallet.disconnect();
              void disconnect();
            }}
          >
            Disconnect
          </button>
        </div>
      )}
    </div>
  );
}
