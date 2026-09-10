// expect: true

// #397: Request/Response.formData() must parse a multipart/form-data body
// into a real FormData - text fields as strings, file-shaped parts
// (a Content-Disposition filename) as real Blob instances (#396) whose
// .type comes from the part's own Content-Type, not the outer body's.
const boundary = "----paseratiTestBoundary";
const body =
  `--${boundary}\r\n` +
  `Content-Disposition: form-data; name="field1"\r\n\r\n` +
  `value1\r\n` +
  `--${boundary}\r\n` +
  `Content-Disposition: form-data; name="file1"; filename="a.txt"\r\n` +
  `Content-Type: text/plain\r\n\r\n` +
  `hello file\r\n` +
  `--${boundary}--\r\n`;
const contentType = `multipart/form-data; boundary=${boundary}`;

async function run(): Promise<boolean> {
  const res = new Response(body, { headers: { "Content-Type": contentType } });
  const resFd = await res.formData();
  const file = resFd.get("file1");
  const check1 =
    resFd.get("field1") === "value1" &&
    file instanceof Blob &&
    file.type === "text/plain" &&
    file.size === 10 &&
    (await file.text()) === "hello file";

  const req = new Request("http://example.com", {
    method: "POST",
    body: body,
    headers: { "Content-Type": contentType },
  });
  const reqFd = await req.formData();
  const check2 = reqFd.get("field1") === "value1";

  // formData() must reject a body whose Content-Type isn't multipart/form-data.
  let check3 = false;
  try {
    const badRes = new Response("hi", {
      headers: { "Content-Type": "text/plain" },
    });
    await badRes.formData();
  } catch (e) {
    check3 = true;
  }

  return check1 && check2 && check3;
}

await run();
