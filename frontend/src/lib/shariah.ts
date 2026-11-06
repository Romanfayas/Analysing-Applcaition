/** Plain-language Shariah status. The raw codes (FAIL, DATA_INSUFFICIENT, UNKNOWN) stay in the API; people read these. */
export type ShariahVariant = 'pass' | 'fail' | 'neutral';

export interface ShariahLabel {
  text: string;
  variant: ShariahVariant;
  /** One-line reason a person can act on. */
  hint: string;
}

export function shariahLabel(status?: string | null): ShariahLabel {
  switch (status) {
    case 'PASS':
      return { text: 'Compliant', variant: 'pass', hint: 'Business is permissible and every financial ratio is within its limit (from reported figures; confirm with your scholar).' };
    case 'FAIL':
      return { text: 'Not compliant', variant: 'fail', hint: 'Fails the screen: a prohibited business, or a financial ratio over its limit.' };
    default:
      return { text: 'Not evaluated', variant: 'neutral', hint: 'Not enough reported figures to decide; no result is assumed.' };
  }
}
