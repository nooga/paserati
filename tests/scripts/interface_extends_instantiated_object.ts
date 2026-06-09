// expect: 1
interface AwaitableString extends Promise<string> {}
declare var value: AwaitableString;

async function read(): Promise<void> {
  await value;
}

1;
