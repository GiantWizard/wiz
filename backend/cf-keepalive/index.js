export default {
  async scheduled(event, env, ctx) {
    const KOYEB_URL = "https://satisfied-filippa-giantwizard-5851d00d.koyeb.app/";

    try {
      const res = await fetch(KOYEB_URL, {
        method: "GET",
        headers: { "User-Agent": "cf-worker-keepalive/1.0" },
      });

      // Check if the response was successful
      if (res.ok) {
        // res.ok is true if the status code is 200-299
        console.log(`Successfully pinged Koyeb: ${res.status} ${res.statusText}`);
      } else {
        // The server responded, but with an error status code
        console.warn(`Ping to Koyeb failed with status: ${res.status} ${res.statusText}`);
      }
    } catch (err) {
      // The fetch itself failed (e.g., network error, DNS issue)
      console.error("Failed to execute fetch to Koyeb:", err);
    }
  },
};