import type { Estimate, Reason } from 'vitals-web';

const REASON_TEXT: Record<Reason, string> = {
  no_face: 'Move into the frame',
  too_much_motion: 'Hold still',
  low_light: 'More light, please',
  talking: 'Try not to talk',
  warming_up: 'Getting a steady signal…',
};

export function reasonText(reason: Reason | null): string {
  return reason === null ? '' : REASON_TEXT[reason];
}

export function instructionText(estimate: Estimate | null): string {
  if (estimate === null) return '';
  if (estimate.reason === null && estimate.confidence === 'invalid') return REASON_TEXT.warming_up;
  return reasonText(estimate.reason);
}
