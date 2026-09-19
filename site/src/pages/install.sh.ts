import installer from "../../../install.sh?raw";

export const prerender = true;

export function GET() {
  return new Response(installer, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=300",
    },
  });
}
