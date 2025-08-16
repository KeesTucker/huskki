// ECU DID logger — Serial only (initial snapshot, then change-only)
// Uses autowp/arduino-mcp2515. ISO-TP + UDS (0x22, 0x3E, 0x27).
// Output rows to Serial (CSV): millis,DID,data_hex
//
// Depends on your did_list.h providing:
//   extern const uint16_t DID_LIST[] PROGMEM;
//   extern const size_t   DID_COUNT;

#include <SPI.h>
#include <mcp2515.h>
#include "did_list.h"

#define LOG_ONLY_ON_CHANGE 1   // after first snapshot, only log when payload changes

// ===== Pins / CAN config =====
#define CAN_CS_PIN   9
#define CAN_SPEED    CAN_500KBPS
#define CAN_CLOCK    MCP_16MHZ      // set to MCP_8MHZ if your MCP2515 crystal is 8 MHz

// ===== UDS / ISO-TP constants =====
static const uint32_t CAN_ID_REQ = 0x7E0;
static const uint32_t CAN_ID_RSP = 0x7E8;

#define SID_DiagnosticSessionControl   0x10
#define SID_TesterPresent              0x3E
#define SID_SecurityAccess             0x27
#define SID_ReadDataByIdentifier       0x22
#define POS_OFFSET                     0x40
#define SUB_ExtendedSession            0x03

#define SA_L2_RequestSeed              0x03
#define SA_L2_SendKey                  0x04
#define SA_L3_RequestSeed              0x05
#define SA_L3_SendKey                  0x06

const unsigned long TESTER_PRESENT_PERIOD_MS = 2000;
const unsigned long FAST_GAP_MS  = 10;   // between FAST polls
const unsigned long SLOW_GAP_MS  = 20;  // between SLOW polls (full list)

#define MIN(a,b) ((a)<(b)?(a):(b))

// ===== FAST list (short = effectively faster polling) =====
const uint16_t FAST_DIDS[] PROGMEM = {
  0x0100, // RPM (raw/4)
  0x0009, // Coolant °C (raw 1:1)
  0x0076, // TPS? (0..1023)
  0x0070, // Grip (0..255)
  0x0001, // Throttle? (0..255)
  0x0031, // Gear enum
  0x0064, // Switch (second byte toggles)
  0x0110, // Injection Time Cyl #1
  0x0012, // O2 Voltage Rear
  0x0042, // Side stand
  0x0003, // IAP Cyl #1
  0x0041, // Clutch
  0x0102, // O2 compensation #1
  0x0108, // Ignition Cyl #1 Coil #2
  0x0132, // Dwell time Cyl #1 Coil #2
  0x0002, // IAP Cyl #1 Voltage
  0x0130, // Dwell time Cyl #1 Coil #1
};
const size_t FAST_COUNT = sizeof(FAST_DIDS)/sizeof(FAST_DIDS[0]);

// ===== Globals =====
MCP2515 mcp2515(CAN_CS_PIN);
struct can_frame rxFrame, txFrame;

unsigned long lastTP = 0;
unsigned long lastFastReq = 0, lastSlowReq = 0;
size_t fastIndex = 0, slowIndex = 0;

// Per-DID change tracking (FAST and SLOW)
static uint8_t  lastChkFast[FAST_COUNT];
static uint8_t  lastLenFast[FAST_COUNT];
static bool     loggedOnceFast[FAST_COUNT];

static uint8_t  lastChkSlow[DID_COUNT];
static uint8_t  lastLenSlow[DID_COUNT];
static bool     loggedOnceSlow[DID_COUNT];

// ===== Helpers =====
bool isFastDid(uint16_t did, size_t* idxOut=nullptr) {
  for (size_t i = 0; i < FAST_COUNT; i++) {
    uint16_t d; memcpy_P(&d, &FAST_DIDS[i], sizeof(uint16_t));
    if (d == did) { if (idxOut) *idxOut = i; return true; }
  }
  return false;
}

// ===== CAN I/O =====
bool sendRaw(uint32_t id, const uint8_t* data, uint8_t len) {
  txFrame.can_id  = id;
  txFrame.can_dlc = len;
  memcpy(txFrame.data, data, len);
  return mcp2515.sendMessage(&txFrame) == MCP2515::ERROR_OK;
}

bool recvRaw(uint32_t &id, uint8_t &len, uint8_t *out, unsigned long timeout_ms=500) {
  unsigned long t0 = millis();
  while (millis() - t0 < timeout_ms) {
    if (mcp2515.readMessage(&rxFrame) == MCP2515::ERROR_OK) {
      id  = rxFrame.can_id;
      len = rxFrame.can_dlc;
      memcpy(out, rxFrame.data, len);
      return true;
    }
  }
  return false;
}

// ===== ISO-TP =====
static uint8_t lastFC_BlockSize = 0;
static uint8_t lastFC_STmin     = 0;

static inline void stminDelay(uint8_t st) {
  if (st <= 0x7F) delay(st);
  else if (st >= 0xF1 && st <= 0xF9) delayMicroseconds((st - 0xF0) * 100);
  else delay(1);
}

bool isotpSend(uint32_t id, const uint8_t* payload, uint16_t size) {
  if (size <= 7) {
    uint8_t f[8] = {0};
    f[0] = 0x00 | (uint8_t)size;
    memcpy(&f[1], payload, size);
    return sendRaw(id, f, 1 + size);
  }
  // First Frame
  uint8_t ff[8] = {0};
  ff[0] = 0x10 | ((size >> 8) & 0x0F);
  ff[1] = size & 0xFF;
  memcpy(&ff[2], payload, 6);
  if (!sendRaw(id, ff, 8)) return false;

  // Wait Flow Control
  uint32_t rid; uint8_t rlen; uint8_t r[8];
  unsigned long t0 = millis();
  while (millis() - t0 < 1000) {
    if (!recvRaw(rid, rlen, r, 1000)) continue;
    if (rid == CAN_ID_RSP && rlen >= 3 && (r[0] & 0xF0) == 0x30) {
      lastFC_BlockSize = r[1];
      lastFC_STmin     = r[2];
      break;
    }
  }

  // Consecutive Frames
  uint16_t idx = 6; uint8_t sn = 1; uint8_t sentInBlock = 0;
  while (idx < size) {
    uint8_t cf[8] = {0};
    cf[0] = 0x20 | (sn & 0x0F);
    uint8_t chunk = (uint8_t)MIN(7, size - idx);
    memcpy(&cf[1], payload + idx, chunk);
    if (!sendRaw(id, cf, 1 + chunk)) return false;
    idx += chunk; sn = (sn + 1) & 0x0F; sentInBlock++;

    if (lastFC_BlockSize > 0 && sentInBlock >= lastFC_BlockSize && idx < size) {
      t0 = millis();
      while (millis() - t0 < 1000) {
        if (!recvRaw(rid, rlen, r, 1000)) continue;
        if (rid == CAN_ID_RSP && rlen >= 3 && (r[0] & 0xF0) == 0x30) {
          lastFC_BlockSize = r[1];
          lastFC_STmin     = r[2];
          break;
        }
      }
      sentInBlock = 0;
    }
    stminDelay(lastFC_STmin);
  }
  return true;
}

bool isotpRecv(uint32_t expectId, uint8_t* out, uint16_t &outLen, uint16_t maxLen, unsigned long timeout_ms=1500) {
  uint32_t id; uint8_t len; uint8_t f[8];
  unsigned long t0 = millis();
  while (millis() - t0 < timeout_ms) {
    if (!recvRaw(id, len, f, timeout_ms)) continue;
    if (id != expectId) continue;

    uint8_t pci = f[0] & 0xF0;
    if (pci == 0x00) {
      uint8_t L = f[0] & 0x0F;
      if (L > 7 || L > (len - 1) || L > maxLen) return false;
      memcpy(out, &f[1], L); outLen = L; return true;
    } else if (pci == 0x10) {
      uint16_t total = ((f[0] & 0x0F) << 8) | f[1];
      if (total > maxLen) return false;
      uint16_t copied = MIN(len - 2, total);
      memcpy(out, &f[2], copied);
      uint16_t pos = copied;

      uint8_t fc[8] = {0x30,0x00,0x00,0,0,0,0,0};
      if (!sendRaw(CAN_ID_REQ, fc, 3)) return false;

      uint8_t expectSN = 1; unsigned long t1 = millis();
      while (pos < total && (millis() - t1) < timeout_ms) {
        if (!recvRaw(id, len, f, timeout_ms)) continue;
        if (id != expectId) continue;
        if ((f[0] & 0xF0) != 0x20) continue;
        uint8_t sn = (f[0] & 0x0F);
        if (sn != expectSN) return false;
        uint8_t chunk = MIN(len - 1, total - pos);
        memcpy(out + pos, &f[1], chunk);
        pos += chunk; expectSN = (expectSN + 1) & 0x0F;
      }
      if (pos == total) { outLen = total; return true; }
      return false;
    }
  }
  return false;
}

// ===== UDS helpers =====
bool udsRequest(const uint8_t* req, uint16_t reqLen, uint8_t* rsp, uint16_t &rspLen, uint16_t maxLen, unsigned long timeout_ms=1500) {
  if (!isotpSend(CAN_ID_REQ, req, reqLen)) return false;

  unsigned long t0 = millis();
  while (millis() - t0 < timeout_ms) {
    if (!isotpRecv(CAN_ID_RSP, rsp, rspLen, maxLen, timeout_ms)) continue;
    if (rspLen >= 3 && rsp[0] == 0x7F && rsp[2] == 0x78) { t0 = millis(); continue; } // response pending
    if (rspLen >= 3 && rsp[0] == 0x7F) return false;
    return true;
  }
  return false;
}

void k01_generateKey(uint8_t level, const uint8_t seed[2], uint8_t key[2]) {
  uint16_t magic = (level == 3) ? 0x6F31 : 0x4D4E; // from your repo
  uint16_t s = ((uint16_t)seed[0] << 8) | seed[1];
  uint16_t k = (uint16_t)((magic * s) & 0xFFFF);
  key[0] = (uint8_t)(k >> 8);
  key[1] = (uint8_t)(k & 0xFF);
}

bool securityAccessLevel(uint8_t level) {
  uint8_t reqSub = (level == 3) ? SA_L3_RequestSeed : SA_L2_RequestSeed;
  uint8_t keySub = (level == 3) ? SA_L3_SendKey     : SA_L2_SendKey;

  // Request seed
  uint8_t req[] = { SID_SecurityAccess, reqSub };
  uint8_t rsp[32]; uint16_t rlen = 0;
  if (!udsRequest(req, sizeof(req), rsp, rlen, sizeof(rsp))) return false;
  if (rlen < 4 || rsp[0] != (SID_SecurityAccess + POS_OFFSET) || rsp[1] != reqSub) return false;

  uint8_t seed[2] = { rsp[2], rsp[3] };
  uint8_t key[2]; k01_generateKey(level, seed, key);
  delay(100);

  // Send key (retry a couple of times)
  for (uint8_t attempt = 0; attempt < 3; attempt++) {
    uint8_t kreq[] = { SID_SecurityAccess, keySub, key[0], key[1] };
    uint8_t krsp[16]; uint16_t klen = 0;
    if (udsRequest(kreq, sizeof(kreq), krsp, klen, sizeof(krsp))) {
      if (klen >= 2 && krsp[0] == (SID_SecurityAccess + POS_OFFSET) && krsp[1] == keySub) return true;
    } else {
      delay(1000);
    }
  }
  return false;
}

bool testerPresent() {
  uint8_t req[] = { SID_TesterPresent, 0x00 };
  isotpSend(CAN_ID_REQ, req, sizeof(req)); // fire-and-forget
  return true;
}

uint16_t readDID(uint16_t did, uint8_t* out, uint16_t maxLen) {
  uint8_t req[3] = { SID_ReadDataByIdentifier, (uint8_t)(did >> 8), (uint8_t)(did & 0xFF) };
  uint8_t rsp[64]; uint16_t rlen = 0;
  if (!udsRequest(req, sizeof(req), rsp, rlen, sizeof(rsp), 1500)) return 0;

  if (rlen >= 3 && rsp[0] == (SID_ReadDataByIdentifier + POS_OFFSET) &&
      rsp[1] == (uint8_t)(did >> 8) && rsp[2] == (uint8_t)(did & 0xFF)) {
    uint16_t dataLen = rlen - 3;
    if (dataLen > maxLen) dataLen = maxLen;
    memcpy(out, &rsp[3], dataLen);
    return dataLen;
  }
  return 0;
}

// --- CRC-8-CCITT (poly 0x07), init 0x00
static inline uint8_t crc8_ccitt_update(uint8_t crc, uint8_t b) {
  crc ^= b;
  for (uint8_t i = 0; i < 8; i++) {
    crc = (crc & 0x80) ? ((crc << 1) ^ 0x07) : (crc << 1);
  }
  return crc;
}
static inline uint8_t crc8_ccitt_buf(uint8_t crc, const uint8_t* p, size_t n) {
  while (n--) crc = crc8_ccitt_update(crc, *p++);
  return crc;
}

void sendFrame(uint16_t did, const uint8_t* data, uint8_t len) {
  uint32_t ms = millis();

  // Build header exactly as Go expects
  uint8_t hdr[7];
  hdr[0] = (uint8_t)(ms);
  hdr[1] = (uint8_t)(ms >> 8);
  hdr[2] = (uint8_t)(ms >> 16);
  hdr[3] = (uint8_t)(ms >> 24);
  hdr[4] = (uint8_t)(did >> 8);   // DID big-endian
  hdr[5] = (uint8_t)(did);
  hdr[6] = len;

  // Compute CRC over millis + DID + len + data (NOT the magic)
  uint8_t crc = 0x00;
  crc = crc8_ccitt_buf(crc, hdr, 4);         // millis (4 LE)
  crc = crc8_ccitt_update(crc, hdr[4]);        // DID hi
  crc = crc8_ccitt_update(crc, hdr[5]);        // DID lo
  crc = crc8_ccitt_update(crc, hdr[6]);        // len
  crc = crc8_ccitt_buf(crc, data, len);      // payload

  // Write the frame bytes (no newlines, no prints)
  Serial.write(0xAA);
  Serial.write(0x55);
  Serial.write(hdr, 7);
  Serial.write(data, len);
  Serial.write(crc);
}

// ===== Setup / Loop =====
void setup() {
  Serial.begin(115200);

  // Keep SPI master mode (UNO/Nano): pin 10 as OUTPUT
  pinMode(10, OUTPUT);

  // Only MCP2515 on SPI now
  pinMode(CAN_CS_PIN, OUTPUT);
  // No forced HIGH here (legacy behavior): let library handle CS

  mcp2515.reset();
  mcp2515.setBitrate(CAN_SPEED, CAN_CLOCK);
  mcp2515.setNormalMode();

  // Try to unlock (best-effort)
  (void)securityAccessLevel(2);
  (void)securityAccessLevel(3);

  lastTP = lastFastReq = lastSlowReq = millis();
}

void pollOne(uint16_t did, uint8_t* lastChkArr, uint8_t* lastLenArr, bool* loggedOnceArr, size_t idx) {
  uint8_t data[64];
  uint16_t len = readDID(did, data, sizeof(data));
  if (len == 0) return;

  // simple checksum over payload
  uint8_t chk = 0; for (uint16_t i = 0; i < len; i++) chk ^= data[i];

  // Always log the first time we successfully read this DID
  if (!loggedOnceArr[idx]) {
    sendFrame(did, data, len);
    loggedOnceArr[idx] = true;
    lastChkArr[idx] = chk; lastLenArr[idx] = len;
    return;
  }

#if LOG_ONLY_ON_CHANGE
  bool changed = (chk != lastChkArr[idx]) || (len != lastLenArr[idx]);
  if (changed) {
    sendFrame(did, data, len);
    lastChkArr[idx] = chk; lastLenArr[idx] = len;
  }
#else
  sendFrame(did, data, len);
  lastChkArr[idx] = chk; lastLenArr[idx] = len;
#endif
}

void loop() {
  unsigned long now = millis();
  if (now - lastTP >= TESTER_PRESENT_PERIOD_MS) { testerPresent(); lastTP = now; }

  // FAST round-robin
  if (now - lastFastReq >= FAST_GAP_MS) {
    uint16_t did; memcpy_P(&did, &FAST_DIDS[fastIndex], sizeof(uint16_t));
    pollOne(did, lastChkFast, lastLenFast, loggedOnceFast, fastIndex);
    fastIndex = (fastIndex + 1) % FAST_COUNT;
    lastFastReq = now;
  }

  // SLOW round-robin (skip DIDs that are in FAST to avoid duplicate logs)
  if (now - lastSlowReq >= SLOW_GAP_MS) {
    uint16_t did; memcpy_P(&did, &DID_LIST[slowIndex], sizeof(uint16_t));
    size_t dummy;
    if (!isFastDid(did, &dummy)) {
      pollOne(did, lastChkSlow, lastLenSlow, loggedOnceSlow, slowIndex);
    }
    slowIndex = (slowIndex + 1) % DID_COUNT;
    lastSlowReq = now;
  }
}
