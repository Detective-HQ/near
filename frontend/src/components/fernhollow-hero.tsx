"use client";

import React, { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ValleyEngine, ValleyAudioStore } from "../lib/valley-engine";

export function FernhollowHero() {
  const containerRef = useRef<HTMLDivElement>(null);
  const valleyRef = useRef<ValleyEngine | null>(null);
  const audioStoreRef = useRef<ValleyAudioStore | null>(null);

  const [isLoaded, setIsLoaded] = useState(false);
  const [introShown, setIntroShown] = useState(false);
  const [isMuted, setIsMuted] = useState(true);

  useEffect(() => {
    if (!containerRef.current) return;

    const audioStore = new ValleyAudioStore();
    audioStoreRef.current = audioStore;

    const valley = new ValleyEngine(
      containerRef.current,
      () => setIsLoaded(true),
      () => setIntroShown(true)
    );
    valleyRef.current = valley;

    // Background preflight to Go backend
    fetch("/api/v1/health").catch(() => {});

    return () => {
      valley.destroy();
      audioStore.destroy();
    };
  }, []);

  const handleSoundToggle = () => {
    const audioStore = audioStoreRef.current;
    if (!audioStore) return;
    if (!audioStore.isInitialized) {
      audioStore.initAudio();
    }
    const muted = audioStore.toggleMute();
    setIsMuted(muted);
  };

  const handleGoEnter = () => {
    valleyRef.current?.startHoverTransition();
    audioStoreRef.current?.setHoverMuffle(true);
  };

  const handleGoLeave = () => {
    valleyRef.current?.stopHoverTransition();
    audioStoreRef.current?.setHoverMuffle(false);
  };

  return (
    <>
      {/* Interactive 3D Night Valley */}
      <div id="valley" aria-hidden="true">
        <div className="js-canvas-container" ref={containerRef} />
      </div>

      {/* Atmospheric Vignette & Film Noise */}
      <div className="vignette" aria-hidden="true" />
      <div className="noise-pass" aria-hidden="true" />

      {/* Main Hero UI */}
      <div className={`ui ${introShown ? "on" : ""}`} id="ui">
        <header>
          <a className="logo" href="/" aria-label="Fernhollow">
            Fernhollow
          </a>
          <nav aria-label="Main">
            <Link href="/workspace">Work</Link>
            <Link href="/workspace/markets">About</Link>
            <Link href="/workspace/intelligence">Journal</Link>
            <Link className="pill" href="/workspace">
              Let’s talk
            </Link>
          </nav>
        </header>

        <section className="hero">
          <h1>Grown, not built.</h1>
          <p className="cap">
            Rooted in nature.
            <br />
            Built for what’s next.
          </p>
          <Link
            className="go"
            href="/workspace"
            id="go"
            aria-label="See our work"
            onPointerEnter={handleGoEnter}
            onPointerLeave={handleGoLeave}
          >
            <svg viewBox="0 0 24 24">
              <path d="M4 12h16M14 6l6 6-6 6" />
            </svg>
          </Link>
        </section>

        <div className="corner">
          <span>Scroll to wander</span>
          <i />
          <button
            className="sound"
            id="sound"
            aria-pressed={!isMuted}
            aria-label="Ambient sound"
            onClick={handleSoundToggle}
          >
            <svg viewBox="0 0 24 24">
              <path d="M4 9.5v5h3.5L12 18V6L7.5 9.5Z" />
              <path className="w" d="M15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11" />
              <path className="x" d="m16 10 4 4m0-4-4 4" />
            </svg>
          </button>
        </div>
      </div>

      {/* Fullscreen pulsing loader */}
      <div className={`loader ${isLoaded ? "gone" : ""}`} id="loader">
        <span />
      </div>

      {/* Exact CSS from Prompt */}
      <style jsx global>{`
        :root {
          --u: min(calc(100vw / 1600), calc(100vh / 900));
          --cream: #f5efe2;
        }
        * {
          box-sizing: border-box;
          margin: 0;
          padding: 0;
        }
        html,
        body {
          height: 100%;
          background: #000;
          overflow: hidden;
        }
        body {
          color: var(--cream);
          font-family: Manrope, system-ui, sans-serif;
          -webkit-font-smoothing: antialiased;
        }
        a {
          color: inherit;
          text-decoration: none;
        }
        button {
          font: inherit;
          color: inherit;
          background: none;
          border: 0;
          cursor: pointer;
        }
        #valley {
          position: fixed;
          inset: 0;
        }
        .js-canvas-container {
          position: absolute;
          inset: 0;
        }
        .js-canvas-container canvas {
          display: block;
          width: 100% !important;
          height: 100% !important;
        }
        .lil-gui {
          display: none !important;
        }
        .vignette {
          position: fixed;
          inset: 0;
          pointer-events: none;
          background: radial-gradient(circle at center, transparent 40%, rgba(0, 0, 0, 0.7) 100%);
          z-index: 1;
        }
        .noise-pass {
          position: fixed;
          inset: 0;
          pointer-events: none;
          opacity: 0.035;
          background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noiseFilter'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noiseFilter)'/%3E%3C/svg%3E");
          z-index: 1;
        }
        .ui {
          position: fixed;
          inset: 0;
          z-index: 2;
          pointer-events: none;
          opacity: 0;
          transition: opacity 1.6s ease;
        }
        .ui.on {
          opacity: 1;
        }
        .ui a,
        .ui button {
          pointer-events: auto;
        }
        header {
          position: absolute;
          left: 0;
          right: 0;
          top: 0;
          display: flex;
          align-items: center;
          justify-content: space-between;
          padding: calc(24 * var(--u)) calc(44 * var(--u)) 0;
        }
        .logo {
          font-size: calc(26 * var(--u));
          font-weight: 500;
          letter-spacing: -0.02em;
        }
        nav {
          display: flex;
          align-items: center;
          gap: calc(44 * var(--u));
          font-size: calc(14.5 * var(--u));
          font-weight: 500;
          letter-spacing: 0.16em;
          text-transform: uppercase;
        }
        nav a:not(.pill) {
          transition: opacity 0.2s;
        }
        nav a:not(.pill):hover {
          opacity: 0.6;
        }
        .pill {
          display: inline-flex;
          align-items: center;
          height: calc(46 * var(--u));
          padding: 0 calc(26 * var(--u));
          border-radius: 99px;
          background: var(--cream);
          color: #151510;
          transition: transform 0.25s;
        }
        .pill:hover {
          transform: translateY(-1px);
        }
        .hero {
          position: absolute;
          left: 0;
          right: 0;
          top: 50%;
          transform: translateY(-46%);
          display: flex;
          flex-direction: column;
          align-items: center;
          text-align: center;
        }
        h1 {
          font: 400 calc(158 * var(--u)) / 0.92 Antonio, "Arial Narrow", sans-serif;
          letter-spacing: -0.012em;
          white-space: nowrap;
        }
        .cap {
          margin-top: calc(48 * var(--u));
          font-size: calc(20 * var(--u));
          line-height: 1.3;
        }
        .go {
          display: grid;
          place-items: center;
          margin-top: calc(34 * var(--u));
          width: calc(58 * var(--u));
          height: calc(58 * var(--u));
          border: 1.5px solid var(--cream);
          border-radius: 50%;
          transition: background 0.3s, color 0.3s;
        }
        .go:hover {
          background: var(--cream);
          color: #151510;
        }
        .go svg {
          width: calc(22 * var(--u));
          height: calc(22 * var(--u));
          fill: none;
          stroke: currentColor;
          stroke-width: 1.6;
          stroke-linecap: round;
          stroke-linejoin: round;
        }
        .corner {
          position: absolute;
          right: calc(44 * var(--u));
          bottom: calc(44 * var(--u));
          display: flex;
          align-items: center;
          gap: calc(18 * var(--u));
          font-size: calc(12.5 * var(--u));
          font-weight: 500;
          letter-spacing: 0.16em;
          text-transform: uppercase;
        }
        .corner i {
          width: 1px;
          height: calc(26 * var(--u));
          background: rgba(245, 239, 226, 0.45);
        }
        .sound {
          display: grid;
          place-items: center;
          width: calc(28 * var(--u));
          height: calc(28 * var(--u));
        }
        .sound svg {
          width: 100%;
          height: 100%;
          fill: none;
          stroke: currentColor;
          stroke-width: 1.5;
          stroke-linecap: round;
          stroke-linejoin: round;
        }
        .sound .w {
          transition: opacity 0.3s;
        }
        .sound[aria-pressed="false"] .w {
          opacity: 0;
        }
        .sound[aria-pressed="true"] .x {
          opacity: 0;
        }
        .loader {
          position: fixed;
          inset: 0;
          z-index: 3;
          display: grid;
          place-items: center;
          background: #000;
          transition: opacity 1s ease;
          pointer-events: none;
        }
        .loader.gone {
          opacity: 0;
        }
        .loader span {
          width: calc(9 * var(--u));
          height: calc(9 * var(--u));
          min-width: 6px;
          min-height: 6px;
          border-radius: 50%;
          background: var(--cream);
          animation: pulse 1.2s ease-in-out infinite;
        }
        @keyframes pulse {
          50% {
            transform: scale(0.4);
            opacity: 0.4;
          }
        }
        a:focus-visible,
        button:focus-visible {
          outline: 2px solid var(--cream);
          outline-offset: 3px;
        }
        @media (max-width: 820px) {
          :root {
            --u: 1px;
          }
          header {
            padding: 18px 18px 0;
          }
          nav a:not(.pill) {
            display: none;
          }
          .pill {
            height: 40px;
            padding: 0 18px;
            font-size: 12px;
          }
          .hero {
            left: 20px;
            right: 20px;
          }
          .corner {
            left: 20px;
            justify-content: center;
            right: 20px;
            bottom: 30px;
            font-size: 11px;
          }
          h1 {
            font-size: clamp(64px, 19vw, 96px);
            white-space: normal;
          }
          .cap {
            font-size: 17px;
          }
        }
        @media (prefers-reduced-motion: reduce) {
          .loader span {
            animation: none;
          }
        }
      `}</style>
    </>
  );
}
