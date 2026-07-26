import JSEncrypt from "jsencrypt"

// The login payload contract (web/api/auth.go + lib/crypt/tls.go): the browser
// RSA-encrypts {"n":nonce,"t":unixMillis,"p":password} with the server's
// public key, PKCS#1 v1.5, and sends it base64-encoded. The nonce is
// single-use and the timestamp is checked in secure mode, which is what makes
// a captured blob worthless to a replayer.

export function encryptLoginPayload(publicKeyPem: string, nonce: string, password: string, serverTimeOffset = 0): string {
  const payload = JSON.stringify({
    n: nonce,
    t: Date.now() + serverTimeOffset,
    p: password,
  })
  const enc = new JSEncrypt()
  enc.setPublicKey(publicKeyPem)
  const cipher = enc.encrypt(payload)
  if (!cipher) {
    throw new Error("RSA encryption failed — the server's public key could not be used")
  }
  return cipher
}

// solvePoW finds x such that sha256(password || x) has `bits` leading zero
// bits, matching lib/common.ValidatePoW. Runs on the main thread in small
// async batches so the UI stays responsive; difficulties in nps.conf are
// low enough (default 16 bits) that a worker would be overkill.
export async function solvePoW(bits: number, password: string): Promise<string> {
  const encoder = new TextEncoder()
  const fullBytes = Math.floor(bits / 8)
  const remBits = bits % 8
  const mask = remBits > 0 ? (0xff << (8 - remBits)) & 0xff : 0

  for (let x = 0; ; x++) {
    const candidate = String(x)
    const digest = new Uint8Array(
      await crypto.subtle.digest("SHA-256", encoder.encode(password + candidate)),
    )
    let ok = true
    for (let i = 0; i < fullBytes; i++) {
      if (digest[i] !== 0) {
        ok = false
        break
      }
    }
    if (ok && remBits > 0 && (digest[fullBytes] & mask) !== 0) {
      ok = false
    }
    if (ok) return candidate

    // Yield every so often so a long search cannot freeze the tab.
    if (x % 512 === 511) {
      await new Promise((resolve) => setTimeout(resolve, 0))
    }
  }
}
