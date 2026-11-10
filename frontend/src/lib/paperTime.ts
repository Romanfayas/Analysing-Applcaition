/** Helpers shared by the paper-trading terminal and the what-if simulator. */

export const SOURCE_LABEL: Record<string, string> = {
  INTRADAY_1M: '1-min bar',
  INTRADAY_5M: '5-min bar',
  EXCHANGE_DAILY_CLOSE: 'NSE official close',
  VENDOR_DAILY_CLOSE: 'vendor daily close',
  LIVE_LATEST: 'latest stored price (the bar time is shown; it is not a live quote when the market is closed)',
  PLAN_FILL_EXCHANGE_DAILY: 'automatic plan fill at the NSE daily bar level',
};

/** Current Indian market time as the value format of <input type="datetime-local">. */
export function nowIstInput(): string {
  return new Date().toLocaleString('sv-SE', { timeZone: 'Asia/Kolkata' }).slice(0, 16).replace(' ', 'T');
}

/** The server reads zone-less times as IST; send the offset explicitly so the browser's own zone never matters. */
export const asIst = (v: string) => `${v}:00+05:30`;

export const money = (n: number) => `₹${n.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
