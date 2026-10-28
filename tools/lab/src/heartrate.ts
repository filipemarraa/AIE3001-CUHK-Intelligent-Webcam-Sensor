// Reference heart rate from a Bluetooth chest strap or oximeter that exposes the standard Heart Rate Service.
// Web Bluetooth needs Chrome or Edge, a secure context (localhost counts) and a click to choose the device.

export const HEART_RATE_SERVICE = 0x180d;
export const HEART_RATE_MEASUREMENT = 0x2a37;

const HR_UINT16 = 0x01;
const CONTACT_DETECTED = 0x02;
const CONTACT_SUPPORTED = 0x04;
const ENERGY_EXPENDED = 0x08;
const RR_INTERVALS = 0x10;

export interface HeartRateMeasurement {
  hrBpm: number;
  // null when the sensor does not report whether it touches the skin.
  contact: boolean | null;
  // Beat-to-beat intervals, oldest first; the device sends them in 1/1024 s.
  rrMs: number[];
}

// Parses the Heart Rate Measurement characteristic (0x2A37) of the Bluetooth Heart Rate Service.
export function parseHeartRateMeasurement(data: DataView): HeartRateMeasurement {
  const tooShort = (): Error => new Error(`heart rate measurement too short (${String(data.byteLength)} bytes)`);
  if (data.byteLength < 1) throw tooShort();
  const flags = data.getUint8(0);
  let at = 1;
  let hrBpm: number;
  if ((flags & HR_UINT16) !== 0) {
    if (data.byteLength < at + 2) throw tooShort();
    hrBpm = data.getUint16(at, true);
    at += 2;
  } else {
    if (data.byteLength < at + 1) throw tooShort();
    hrBpm = data.getUint8(at);
    at += 1;
  }
  if ((flags & ENERGY_EXPENDED) !== 0) {
    if (data.byteLength < at + 2) throw tooShort();
    at += 2;
  }
  const rrMs: number[] = [];
  if ((flags & RR_INTERVALS) !== 0) {
    for (; at + 2 <= data.byteLength; at += 2) rrMs.push((data.getUint16(at, true) * 1000) / 1024);
  }
  const contact = (flags & CONTACT_SUPPORTED) === 0 ? null : (flags & CONTACT_DETECTED) !== 0;
  return { hrBpm, contact, rrMs };
}

// The part of Web Bluetooth the lab uses; TypeScript's DOM library does not declare it.
interface Characteristic extends EventTarget {
  value?: DataView;
  startNotifications(): Promise<Characteristic>;
}
interface Device extends EventTarget {
  name?: string;
  gatt?: {
    connected: boolean;
    connect(): Promise<{
      getPrimaryService(s: number): Promise<{ getCharacteristic(c: number): Promise<Characteristic> }>;
    }>;
    disconnect(): void;
  };
}
interface Bluetooth {
  requestDevice(options: { filters: { services: number[] }[] }): Promise<Device>;
}

export function bluetoothAvailable(): boolean {
  return window.isSecureContext && 'bluetooth' in navigator;
}

export interface HeartRateSensor {
  name: string;
  disconnect(): void;
}

// Asks the user for a heart rate sensor (call from a click) and calls onMeasurement on every notification.
export async function connectHeartRateSensor(
  onMeasurement: (m: HeartRateMeasurement) => void,
  onDisconnect: () => void,
): Promise<HeartRateSensor> {
  const bluetooth = (navigator as Navigator & { bluetooth?: Bluetooth }).bluetooth;
  if (bluetooth === undefined) throw new Error('Web Bluetooth is not available in this browser');
  const device = await bluetooth.requestDevice({ filters: [{ services: [HEART_RATE_SERVICE] }] });
  if (device.gatt === undefined) throw new Error('the device has no GATT server');
  const server = await device.gatt.connect();
  const service = await server.getPrimaryService(HEART_RATE_SERVICE);
  const characteristic = await service.getCharacteristic(HEART_RATE_MEASUREMENT);
  characteristic.addEventListener('characteristicvaluechanged', () => {
    if (characteristic.value === undefined) return;
    try {
      onMeasurement(parseHeartRateMeasurement(characteristic.value));
    } catch {
      // A malformed notification is dropped; the next one arrives within a second.
    }
  });
  device.addEventListener('gattserverdisconnected', onDisconnect);
  await characteristic.startNotifications();
  const gatt = device.gatt;
  return {
    name: device.name ?? 'unnamed sensor',
    disconnect: () => {
      device.removeEventListener('gattserverdisconnected', onDisconnect);
      if (gatt.connected) gatt.disconnect();
    },
  };
}
