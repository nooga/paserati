type TypeOf<T> = T extends { _o: infer O } ? O : never;
type Num = number;
interface Box { v: string }
export type { TypeOf as infer, Num as default, Box };
export type Direct = boolean;
export const marker = 1;
