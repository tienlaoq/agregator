"use client";

/**
 * Короткий двухтоновый сигнал (Web Audio API, без отдельного файла).
 * Общий для чата и колокольчика: один AudioContext на вкладку, поэтому
 * разблокировка по первому жесту пользователя включает звук для обеих подсистем.
 * Браузеры часто блокируют звук до первого жеста — см. unlockNotificationAudio.
 */

let ctxRef: AudioContext | null = null;

function audioCtx(): AudioContext | null {
  const Ctx =
    window.AudioContext ??
    (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!Ctx) return null;
  if (!ctxRef) ctxRef = new Ctx();
  return ctxRef;
}

export async function unlockNotificationAudio(): Promise<void> {
  try {
    const ctx = audioCtx();
    if (!ctx) return;
    if (ctx.state === "suspended") await ctx.resume();
  } catch {
    // ignore
  }
}

export function playNotificationSound(): void {
  try {
    const ctx = audioCtx();
    if (!ctx) return;
    if (ctx.state === "suspended") void ctx.resume();

    const t0 = ctx.currentTime;
    const gain = ctx.createGain();
    gain.connect(ctx.destination);
    gain.gain.setValueAtTime(0, t0);
    gain.gain.linearRampToValueAtTime(0.12, t0 + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.001, t0 + 0.22);

    const o1 = ctx.createOscillator();
    o1.type = "sine";
    o1.frequency.setValueAtTime(880, t0);
    o1.connect(gain);
    o1.start(t0);
    o1.stop(t0 + 0.11);

    const o2 = ctx.createOscillator();
    o2.type = "sine";
    o2.frequency.setValueAtTime(660, t0 + 0.1);
    o2.connect(gain);
    o2.start(t0 + 0.1);
    o2.stop(t0 + 0.22);
  } catch {
    // ignore playback errors (autoplay policy, etc.)
  }
}
