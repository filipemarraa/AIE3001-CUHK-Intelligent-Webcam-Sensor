import type { Point } from '../worker/vision/roi.js';
import { CANONICAL_FACE } from './canonical-face.js';

export interface FacePose {
  // Head turn about the vertical axis; positive turns the side on the right of the image away from the camera.
  yawDeg?: number;
  // Head tilt in the image plane, clockwise on screen.
  rollDeg?: number;
  // Face width in pixels, from landmark 234 to landmark 454 when facing the camera.
  widthPx?: number;
  centre?: { x: number; y: number };
  frame?: { width: number; height: number };
}

const CANONICAL_WIDTH = 2 * 7.664182;

// Landmarks of the canonical face, as the Face Landmarker returns them: x and y normalised to the frame, z in units of
// the frame width and smaller towards the camera.
export function faceLandmarks(pose: FacePose = {}): Point[] {
  const { width, height } = pose.frame ?? { width: 640, height: 480 };
  const centre = pose.centre ?? { x: width / 2, y: height / 2 };
  const scale = (pose.widthPx ?? 150) / CANONICAL_WIDTH;
  const yaw = ((pose.yawDeg ?? 0) * Math.PI) / 180;
  const roll = ((pose.rollDeg ?? 0) * Math.PI) / 180;
  return CANONICAL_FACE.map(([x, y, z]) => {
    const tx = x * Math.cos(yaw) + z * Math.sin(yaw);
    const tz = -x * Math.sin(yaw) + z * Math.cos(yaw);
    const sx = tx * scale;
    const sy = -y * scale;
    const rx = sx * Math.cos(roll) - sy * Math.sin(roll);
    const ry = sx * Math.sin(roll) + sy * Math.cos(roll);
    return { x: (centre.x + rx) / width, y: (centre.y + ry) / height, z: (-tz * scale) / width };
  });
}
