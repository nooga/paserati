// expect: TIMEOUT
// tRPC's codes.ts: InvertKeyValue over `as const` numbers (signed computed
// keys, numeric mapped keys, typeof in hoisted aliases and annotations) and
// over Record<K, number> (index signatures through keyof and T[string | number]).
type KeyFromValue<TValue, TType extends Record<PropertyKey, PropertyKey>> = {
  [K in keyof TType]: TValue extends TType[K] ? K : never;
}[keyof TType];
type InvertKeyValue<TType extends Record<PropertyKey, PropertyKey>> = {
  [TValue in TType[keyof TType]]: KeyFromValue<TValue, TType>;
};
type ValueOf<T> = T[keyof T];
const BY_KEY = { PARSE_ERROR: -32700, TIMEOUT: -32008 } as const;
export type CODE = ValueOf<typeof BY_KEY>;
const BY_NUMBER: InvertKeyValue<typeof BY_KEY> = {
  [-32700]: 'PARSE_ERROR',
  [-32008]: 'TIMEOUT',
};
const HTTP: Record<keyof typeof BY_KEY, number> = { PARSE_ERROR: 400, TIMEOUT: 408 };
const BY_HTTP: InvertKeyValue<typeof HTTP> = { 400: 'PARSE_ERROR', 408: 'TIMEOUT' };
const code: CODE = -32008;
BY_HTTP[408] === BY_NUMBER[code] ? BY_NUMBER[code] : "no";
