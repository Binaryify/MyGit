// Serves the site at binaryify.com/mygit: the files of this folder, found
// under the path past /mygit. Everything else on the domain is left alone.
const BASE = "/mygit";

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    // The pages' links are relative: they need the trailing slash.
    if (url.pathname === BASE) {
      url.pathname = BASE + "/";
      return Response.redirect(url.toString(), 301);
    }
    if (!url.pathname.startsWith(BASE + "/")) {
      return new Response("Not found", { status: 404 });
    }
    url.pathname = url.pathname.slice(BASE.length);
    const response = await env.ASSETS.fetch(new Request(url, request));
    return ranged(request, response);
  },
};

// ranged answers a request for part of a file, as Safari asks of a video
// before it plays it, with that part: the assets answer with the whole.
async function ranged(request, response) {
  if (response.status !== 200) return response;
  const range = /^bytes=(\d*)-(\d*)$/.exec(request.headers.get("Range") || "");
  if (!range) {
    const whole = new Response(response.body, response);
    whole.headers.set("Accept-Ranges", "bytes");
    return whole;
  }
  const body = await response.arrayBuffer();
  const size = body.byteLength;
  let start = range[1] === "" ? size - Number(range[2]) : Number(range[1]);
  let end = range[1] === "" || range[2] === "" ? size - 1 : Math.min(Number(range[2]), size - 1);
  if (start < 0) start = 0;
  if (start > end || start >= size) {
    return new Response(null, { status: 416, headers: { "Content-Range": `bytes */${size}` } });
  }
  const headers = new Headers(response.headers);
  headers.set("Content-Range", `bytes ${start}-${end}/${size}`);
  headers.set("Content-Length", String(end - start + 1));
  headers.set("Accept-Ranges", "bytes");
  return new Response(body.slice(start, end + 1), { status: 206, headers });
}
