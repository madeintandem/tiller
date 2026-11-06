// Usage: npm run publish -- <video-id>
// Uploads out/<id>.mp4 with its thumbnail and captions to YouTube, never publicly.
import { publish } from "../publish/youtube.js";

const id = process.argv[2];
if (!id) {
  console.error("usage: npm run publish -- <video-id>");
  process.exit(2);
}
try {
  const youtubeId = await publish(id);
  console.log(`published ${id} as https://youtu.be/${youtubeId}`);
} catch (e) {
  console.error((e as Error).message);
  process.exit(1);
}
