// expect: true

// #395 follow-up: Response.blob()/Request.blob() must resolve to a real
// Blob (correct instanceof/prototype chain), not a bare Uint8Array, and
// its "type" should come from the Content-Type header.
async function run(): Promise<boolean> {
  const res = new Response("hi there", {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
  const resBlob = await res.blob();
  const check1 =
    resBlob instanceof Blob &&
    Object.getPrototypeOf(resBlob) === Blob.prototype &&
    resBlob.type === "text/plain" &&
    resBlob.size === 8;

  const req = new Request("http://example.com", {
    method: "POST",
    body: "abc",
    headers: { "Content-Type": "application/json" },
  });
  const reqBlob = await req.blob();
  const check2 =
    reqBlob instanceof Blob &&
    Object.getPrototypeOf(reqBlob) === Blob.prototype &&
    reqBlob.type === "application/json" &&
    reqBlob.size === 3;

  // Request with no body at all must still resolve to an empty Blob.
  const emptyReq = new Request("http://example.com");
  const emptyBlob = await emptyReq.blob();
  const check3 = emptyBlob instanceof Blob && emptyBlob.size === 0;

  return check1 && check2 && check3;
}

await run();
