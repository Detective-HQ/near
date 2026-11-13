"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ValleyAudioStore, ValleyEngine } from "../lib/valley-engine";
import { LINKS } from "./landing/config";
import "./fernhollow-hero.css";

export function FernhollowHero() {
  const containerRef = useRef<HTMLDivElement>(null);
  const valleyRef = useRef<ValleyEngine | null>(null);
  const audioRef = useRef<ValleyAudioStore | null>(null);

  const [isLoaded, setIsLoaded] = useState(false);
  const [introShown, setIntroShown] = useState(false);
  const [isMuted, setIsMuted] = useState(true);

  useEffect(() => {
    if (!containerRef.current) return;
    const audio = new ValleyAudioStore();
    audioRef.current = audio;
    const valley = new ValleyEngine(
      containerRef.current,
      () => setIsLoaded(true),
      () => setIntroShown(true),
    );
    valleyRef.current = valley;

    // Pause the 3D scene while the hero is scrolled out of view.
    const io = new IntersectionObserver(([entry]) => valley.setPaused(!entry.isIntersecting));
    io.observe(containerRef.current);

    // Warm up the API while the valley loads.
    fetch("/api/v1/health").catch(() => {});

    return () => {
      io.disconnect();
      valley.destroy();
      audio.destroy();
      valleyRef.current = null;
      audioRef.current = null;
    };
  }, []);

  const toggleSound = () => {
    const audio = audioRef.current;
    if (!audio) return;
    // Browsers only allow audio after a user gesture, so start it on the first click.
    if (!audio.isInitialized) audio.initAudio();
    setIsMuted(audio.toggleMute());
  };

  const hoverStart = () => {
    valleyRef.current?.startHoverTransition();
    audioRef.current?.setHoverMuffle(true);
  };
  const hoverEnd = () => {
    valleyRef.current?.stopHoverTransition();
    audioRef.current?.setHoverMuffle(false);
  };

  return (
    <section className="fh" aria-label="Introduction">
      <div className="fh-valley" ref={containerRef} aria-hidden="true" />

      <div className={`fh-ui${introShown ? " on" : ""}`}>
        <header className="fh-header">
          <Link className="fh-logo" href="/" aria-label="Zonyx home">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src="/images/sightline-mark.svg" alt="" width={34} height={34} />
            <span>Zonyx</span>
          </Link>
          <nav className="fh-nav" aria-label="Main">
            <a href="#product">Product</a>
            <Link href={LINKS.trading}>Trading</Link>
            <Link href={LINKS.research}>Research</Link>
            <a href={LINKS.docs} target="_blank" rel="noreferrer">Docs</a>
            <Link className="fh-pill" href={LINKS.app}>
              Launch app
            </Link>
          </nav>
        </header>

        <section className="fh-hero">
          <p className="fh-eyebrow">Cross-chain DeFi · Trading intelligence</p>
          <h1>
            <span className="fh-nb">One interface.</span> <span className="fh-nb">Every chain.</span>
            <br />
            <span className="fh-nb">Evidence you can check.</span>
          </h1>
          <p className="fh-cap">
            Zonyx lets you swap across chains from your own wallet, then shows the market structure, liquidity and data
            behind every signal.
          </p>
          <div className="fh-actions">
            <Link
              className="fh-btn fh-btn-primary"
              href={LINKS.app}
              onPointerEnter={hoverStart}
              onPointerLeave={hoverEnd}
              onFocus={hoverStart}
              onBlur={hoverEnd}
            >
              Launch app
            </Link>
            <a className="fh-btn fh-btn-secondary" href={LINKS.methodology} target="_blank" rel="noreferrer">
              Read the methodology
            </a>
          </div>
        </section>

        <div className="fh-corner">
          <span>Scroll to wander</span>
          <i aria-hidden="true" />
          <button className="fh-sound" type="button" aria-pressed={!isMuted} aria-label="Ambient sound" onClick={toggleSound}>
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M4 9.5v5h3.5L12 18V6L7.5 9.5Z" />
              <path className="w" d="M15.5 9a4 4 0 0 1 0 6M18 6.5a7.5 7.5 0 0 1 0 11" />
              <path className="x" d="m16 10 4 4m0-4-4 4" />
            </svg>
          </button>
        </div>
      </div>

      <div className={`fh-loader${isLoaded ? " gone" : ""}`} aria-hidden="true">
        <span />
      </div>
    </section>
  );
}
