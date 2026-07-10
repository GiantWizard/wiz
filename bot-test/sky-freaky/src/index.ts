import { verifyKey } from 'discord-interactions';
import { APIInteraction } from 'discord-api-types/v10';

export interface Env {
  DISCORD_TOKEN: string;
  DISCORD_PUBLIC_KEY: string;
  DISCORD_APPLICATION_ID: string;
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method !== 'POST') {
      return new Response('Method not allowed.', { status: 405 });
    }

    const signature = request.headers.get('x-signature-ed25519');
    const timestamp = request.headers.get('x-signature-timestamp');
    const body = await request.clone().arrayBuffer();

    const isValidRequest = signature && timestamp &&
      verifyKey(body, signature, timestamp, env.DISCORD_PUBLIC_KEY);

    if (!isValidRequest) {
      console.error('Invalid Request Signature');
      return new Response('Bad request signature.', { status: 401 });
    }

    const interaction: APIInteraction = await request.json();

    if (interaction.type === 1) { // 1 = PING
      console.log('Handling Ping request');
      return new Response(
        JSON.stringify({ type: 1 }), // 1 = PONG
        { headers: { 'Content-Type': 'application/json' } }
      );
    }

    if (interaction.type === 2) { // 2 = APPLICATION_COMMAND
      if ('name' in interaction.data && interaction.data.name.toLowerCase() === 'ping') {
        console.log('Handling ping command');
        return new Response(
          JSON.stringify({
            type: 4, // 4 = CHANNEL_MESSAGE_WITH_SOURCE
            data: { content: 'Pong!' },
          }),
          { headers: { 'Content-Type': 'application/json' } }
        );
      }
    }

    console.error('Unknown interaction type or command');
    return new Response('Unknown interaction type or command.', { status: 400 });
  },
};