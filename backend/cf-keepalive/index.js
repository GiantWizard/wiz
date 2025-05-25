/**
 * A scheduled handler that fires once per minute
 * and issues a GET to your Koyeb service.
 */
export default {
    async scheduled(event, env, ctx) {
      try {
        const res = await fetch(
          "https://wiz-backend-giantwizard-0f2a46ea.koyeb.app/",
          {
            method: "GET",
            headers: { "User-Agent": "cf-keepalive" },
          }
        );
        console.log("Pinged Koyeb:", res.status);
      } catch (err) {
        console.error("Failed to ping Koyeb:", err);
      }
    },
  };
  