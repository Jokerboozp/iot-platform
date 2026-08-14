/* Minimal MQTT 3.1.1 WebSocket client for platform read-only live updates. */
(() => {
  const text = new TextEncoder();
  const stringField = value => {
    const bytes = text.encode(value);
    return Uint8Array.of(bytes.length >> 8, bytes.length & 255, ...bytes);
  };
  const remaining = value => {
    const out = [];
    do {
      let digit = value % 128;
      value = Math.floor(value / 128);
      if (value > 0) digit |= 128;
      out.push(digit);
    } while (value > 0);
    return out;
  };
  const packet = (header, body = new Uint8Array()) => Uint8Array.of(header, ...remaining(body.length), ...body);

  class MQTTWebSocketClient {
    constructor({url, username, password, clientId, onMessage, onState}) {
      Object.assign(this, {url, username, password, clientId, onMessage, onState});
      this.buffer = new Uint8Array();
      this.packetId = 1;
      this.closed = false;
    }
    connect() {
      this.closed = false;
      const ws = this.socket = new WebSocket(this.url, ['mqtt']);
      ws.binaryType = 'arraybuffer';
      ws.onopen = () => {
        const body = Uint8Array.of(...stringField('MQTT'), 4, 0xc2, 0, 30, ...stringField(this.clientId), ...stringField(this.username), ...stringField(this.password));
        ws.send(packet(0x10, body));
      };
      ws.onmessage = event => this.consume(new Uint8Array(event.data));
      ws.onerror = () => this.onState?.('error');
      ws.onclose = () => { this.stopPing(); this.onState?.('disconnected'); };
    }
    consume(chunk) {
      const merged = new Uint8Array(this.buffer.length + chunk.length);
      merged.set(this.buffer); merged.set(chunk, this.buffer.length); this.buffer = merged;
      let offset = 0;
      while (offset + 2 <= this.buffer.length) {
        let multiplier = 1, length = 0, cursor = offset + 1, digit;
        do {
          if (cursor >= this.buffer.length) return;
          digit = this.buffer[cursor++]; length += (digit & 127) * multiplier; multiplier *= 128;
        } while (digit & 128);
        if (cursor + length > this.buffer.length) break;
        this.handle(this.buffer[offset], this.buffer.slice(cursor, cursor + length));
        offset = cursor + length;
      }
      this.buffer = this.buffer.slice(offset);
    }
    handle(header, body) {
      const type = header >> 4;
      if (type === 2) {
        if (body[1] !== 0) { this.onState?.(`rejected:${body[1]}`); this.disconnect(); return; }
        this.onState?.('connected');
        this.ping = setInterval(() => this.send(packet(0xc0)), 20000);
        return;
      }
      if (type !== 3 || body.length < 2) return;
      const topicLength = body[0] * 256 + body[1];
      const topic = new TextDecoder().decode(body.slice(2, 2 + topicLength));
      let payloadOffset = 2 + topicLength;
      if (((header >> 1) & 3) > 0) payloadOffset += 2;
      const payload = new TextDecoder().decode(body.slice(payloadOffset));
      this.onMessage?.(topic, payload);
    }
    subscribe(topics) {
      const id = this.nextPacketId();
      const fields = topics.flatMap(topic => [...stringField(topic), 1]);
      this.send(packet(0x82, Uint8Array.of(id >> 8, id & 255, ...fields)));
    }
    nextPacketId() { this.packetId = this.packetId % 65535 + 1; return this.packetId; }
    send(data) { if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(data); }
    stopPing() { if (this.ping) clearInterval(this.ping); this.ping = null; }
    disconnect() {
      this.closed = true; this.stopPing(); this.send(packet(0xe0)); this.socket?.close();
    }
  }
  window.MQTTWebSocketClient = MQTTWebSocketClient;
})();
