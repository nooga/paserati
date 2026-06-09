// expect: 1
declare function before(): void;
declare function after(value: boolean): void;

async function func(value: boolean): Promise<void> {
  before();
  const result = await Promise.resolve(true) || value;
  after(result);
}

1;
