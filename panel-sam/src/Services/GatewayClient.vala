/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 *
 * GatewayClient — async HTTP client for the claw gateway.
 * POST /v1/chat/completions → returns assistant message content.
 *
 * The indicator keeps one session alive across follow-up questions until the
 * user explicitly starts or resets the session.
 */

public class Sam.GatewayClient : GLib.Object {

    public delegate void StreamUpdateCallback (string kind, string content);

    private const string GATEWAY_URL = "http://127.0.0.1:4389/v1/chat/completions";
    private const string GATEWAY_RUNTIME_URL = "http://127.0.0.1:4389/v1/runtime";
    private const string GATEWAY_OPENAI_CONNECT_URL = "http://127.0.0.1:4389/v1/providers/openai/connect";
    private const string GATEWAY_OPENAI_CODEX_INIT_URL = "http://127.0.0.1:4389/v1/providers/openai-codex/init";
    private const string GATEWAY_OPENAI_CODEX_EXCHANGE_URL = "http://127.0.0.1:4389/v1/providers/openai-codex/exchange";
    private const string OPENAI_CTA_PREFIX = "__GITHUB_RATE_LIMIT__::";

    private Soup.Session session;
    private string current_session_id;
    private string current_model = "gpt-5.4";
    private string current_provider = "github-copilot";

    construct {
        session = new Soup.Session () {
            timeout = 120
        };
        start_new_session ();
    }

    public void start_new_session () {
        current_session_id = "panel-sam-" + GLib.Uuid.string_random ();
    }

    public void reset_session () {
        start_new_session ();
    }

    public bool should_offer_openai_codex (string message) {
        return message.index_of (OPENAI_CTA_PREFIX) >= 0;
    }

    public string normalize_error_message (string message) {
        int prefix_at = message.index_of (OPENAI_CTA_PREFIX);
        if (prefix_at >= 0) {
            return message.substring (prefix_at + OPENAI_CTA_PREFIX.length).strip ();
        }
        return message;
    }

    private string current_request_model () {
        var model = current_model.strip ();
        if (model.length == 0) {
            return "gpt-5.4";
        }

        var parts = model.split ("/");
        return parts[parts.length - 1].strip ();
    }

    private Soup.Message build_chat_message (string query, bool stream) {
        var builder = new Json.Builder ();
        builder.begin_object ();
            builder.set_member_name ("model");
            builder.add_string_value (current_request_model ());
            builder.set_member_name ("stream");
            builder.add_boolean_value (stream);
            if (stream) {
                builder.set_member_name ("x_claw_preview");
                builder.add_boolean_value (true);
            }
            builder.set_member_name ("session_id");
            builder.add_string_value (current_session_id);
            builder.set_member_name ("messages");
            builder.begin_array ();
                builder.begin_object ();
                    builder.set_member_name ("role");
                    builder.add_string_value ("user");
                    builder.set_member_name ("content");
                    builder.add_string_value (query);
                builder.end_object ();
            builder.end_array ();
        builder.end_object ();

        var generator = new Json.Generator ();
        generator.set_root (builder.get_root ());
        string payload = generator.to_data (null);

        var msg = new Soup.Message ("POST", GATEWAY_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take (payload.data)
        );
        return msg;
    }

    private async void refresh_runtime_profile () throws GLib.Error {
        var msg = new Soup.Message ("GET", GATEWAY_RUNTIME_URL);
        GLib.Bytes response_bytes = yield session.send_and_read_async (
            msg, GLib.Priority.DEFAULT, null
        );

        if (msg.status_code != 200) {
            return;
        }

        string body = (string) response_bytes.get_data ();
        var parser = new Json.Parser ();
        parser.load_from_data (body, -1);

        unowned Json.Node root = parser.get_root ();
        if (root == null || root.get_node_type () != Json.NodeType.OBJECT) {
            return;
        }

        unowned Json.Object root_obj = root.get_object ();
        if (root_obj.has_member ("model")) {
            var model = root_obj.get_string_member ("model").strip ();
            if (model.length > 0) {
                current_model = model;
            }
        }
        if (root_obj.has_member ("provider")) {
            var provider = root_obj.get_string_member ("provider").strip ();
            if (provider.length > 0) {
                current_provider = provider;
            }
        }
    }

    public async string begin_openai_codex_subscription () throws GLib.Error {
        var msg = new Soup.Message ("POST", GATEWAY_OPENAI_CODEX_INIT_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take ("{}".data)
        );

        GLib.Bytes response_bytes = yield session.send_and_read_async (
            msg, GLib.Priority.DEFAULT, null
        );

        string body = (string) response_bytes.get_data ();
        if (msg.status_code != 200) {
            throw new GLib.IOError.FAILED (
                "No pude iniciar la conexión con ChatGPT: %s",
                body.strip ()
            );
        }

        var parser = new Json.Parser ();
        parser.load_from_data (body, -1);
        unowned Json.Object root_obj = parser.get_root ().get_object ();
        return root_obj.get_string_member ("authUrl");
    }

    public async void finish_openai_codex_subscription (string raw_url) throws GLib.Error {
        var builder = new Json.Builder ();
        builder.begin_object ();
            builder.set_member_name ("raw_url");
            builder.add_string_value (raw_url.strip ());
        builder.end_object ();

        var generator = new Json.Generator ();
        generator.set_root (builder.get_root ());
        string payload = generator.to_data (null);

        var msg = new Soup.Message ("POST", GATEWAY_OPENAI_CODEX_EXCHANGE_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take (payload.data)
        );

        GLib.Bytes response_bytes = yield session.send_and_read_async (
            msg, GLib.Priority.DEFAULT, null
        );

        string body = (string) response_bytes.get_data ();
        if (msg.status_code != 200) {
            throw new GLib.IOError.FAILED (
                "No pude completar la conexión con ChatGPT: %s",
                body.strip ()
            );
        }

        yield refresh_runtime_profile ();
        reset_session ();
    }

    public async void connect_openai_codex_api_key (string api_key) throws GLib.Error {
        var builder = new Json.Builder ();
        builder.begin_object ();
            builder.set_member_name ("api_key");
            builder.add_string_value (api_key.strip ());
        builder.end_object ();

        var generator = new Json.Generator ();
        generator.set_root (builder.get_root ());
        string payload = generator.to_data (null);

        var msg = new Soup.Message ("POST", GATEWAY_OPENAI_CONNECT_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take (payload.data)
        );

        GLib.Bytes response_bytes = yield session.send_and_read_async (
            msg, GLib.Priority.DEFAULT, null
        );

        string body = (string) response_bytes.get_data ();
        if (msg.status_code != 200) {
            throw new GLib.IOError.FAILED (
                "No pude conectar OpenAI Codex: %s",
                body.strip ()
            );
        }

        yield refresh_runtime_profile ();
        reset_session ();
    }

    private string classify_gateway_error (Soup.Message msg, string err_body) {
        if (msg.status_code == 429) {
            string? error_class = msg.response_headers.get_one ("X-Claw-Error-Class");
            if (error_class == "github-rate-limit") {
                return OPENAI_CTA_PREFIX + err_body;
            }
        }
        return err_body;
    }

    /**
     * Send a user query to the gateway and return the assistant response text.
     * This is an async method — call with chat.begin(query, callback).
     */
    public async string chat (string query) throws GLib.Error {
        try {
            yield refresh_runtime_profile ();
        } catch (GLib.Error e) {
        }

        var msg = build_chat_message (query, false);

        /* Fire async request */
        GLib.Bytes response_bytes = yield session.send_and_read_async (
            msg, GLib.Priority.DEFAULT, null
        );

        /* Handle auth / gateway errors */
        if (msg.status_code == 502) {
            string err_body = (string) response_bytes.get_data ();
            if ("token" in err_body.down ()) {
                throw new GLib.IOError.PERMISSION_DENIED (
                    "GitHub token expired — re-run device login on the VM"
                );
            }
            throw new GLib.IOError.FAILED (
                "Gateway error (502): %s",
                classify_gateway_error (msg, err_body)
            );
        }

        if (msg.status_code != 200) {
            string err_body = (string) response_bytes.get_data ();
            throw new GLib.IOError.FAILED (
                "Gateway returned HTTP %u: %s",
                msg.status_code,
                classify_gateway_error (msg, err_body)
            );
        }

        /* Parse JSON response */
        string body = (string) response_bytes.get_data ();

        var parser = new Json.Parser ();
        parser.load_from_data (body, -1);

        unowned Json.Node root = parser.get_root ();
        unowned Json.Object root_obj = root.get_object ();
        unowned Json.Array choices = root_obj.get_array_member ("choices");
        unowned Json.Object first = choices.get_object_element (0);
        unowned Json.Object message = first.get_object_member ("message");

        return message.get_string_member ("content");
    }

    public async string chat_stream (string query, owned StreamUpdateCallback? on_update = null) throws GLib.Error {
        try {
            yield refresh_runtime_profile ();
        } catch (GLib.Error e) {
        }

        var msg = build_chat_message (query, true);
        GLib.InputStream response_stream = yield session.send_async (
            msg, GLib.Priority.DEFAULT, null
        );

        if (on_update != null) {
            on_update ("phase", "Pensando y preparando respuesta\u2026");
        }

        if (msg.status_code == 502) {
            var err_body = yield read_stream_text (response_stream);
            if ("token" in err_body.down ()) {
                throw new GLib.IOError.PERMISSION_DENIED (
                    "GitHub token expired \u2014 re-run device login on the VM"
                );
            }
            throw new GLib.IOError.FAILED (
                "Gateway error (502): %s",
                classify_gateway_error (msg, err_body)
            );
        }

        if (msg.status_code != 200) {
            var err_body = yield read_stream_text (response_stream);
            throw new GLib.IOError.FAILED (
                "Gateway returned HTTP %u: %s",
                msg.status_code,
                classify_gateway_error (msg, err_body)
            );
        }

        var data_stream = new GLib.DataInputStream (response_stream);
        string full_text = "";
        string stream_error = "";
        bool saw_content = false;

        while (true) {
            size_t line_length = 0;
            string? line = yield data_stream.read_line_utf8_async (
                GLib.Priority.DEFAULT,
                null,
                out line_length
            );
            if (line == null) {
                break;
            }

            if (!line.has_prefix ("data: ")) {
                continue;
            }

            var payload = line.substring (6).strip ();
            if (payload == "[DONE]") {
                break;
            }

            string preview_kind;
            string preview_content;
            if (extract_preview_event (payload, out preview_kind, out preview_content)) {
                if (preview_kind == "preview") {
                    full_text = preview_content;
                    saw_content = preview_content.strip ().length > 0;
                } else if (preview_kind == "error") {
                    stream_error = preview_content;
                }

                if (on_update != null) {
                    on_update (preview_kind, preview_content);
                }
                continue;
            }

            var delta_text = extract_sse_delta_text (payload);
            if (delta_text == null || delta_text.length == 0) {
                continue;
            }

            if (!saw_content) {
                saw_content = true;
                if (on_update != null) {
                    on_update ("phase", "Redactando respuesta\u2026");
                }
            }

            full_text += delta_text;
            if (on_update != null) {
                on_update ("preview", full_text);
            }
        }

        if (stream_error.strip ().length > 0) {
            throw new GLib.IOError.FAILED (stream_error);
        }

        return full_text;
    }

    private async string read_stream_text (GLib.InputStream response_stream) throws GLib.Error {
        var data_stream = new GLib.DataInputStream (response_stream);
        var text = new GLib.StringBuilder ();

        while (true) {
            size_t line_length = 0;
            string? line = yield data_stream.read_line_utf8_async (
                GLib.Priority.DEFAULT,
                null,
                out line_length
            );
            if (line == null) {
                break;
            }
            text.append (line);
            text.append ("\n");
        }

        return text.str.strip ();
    }

    private string? extract_sse_delta_text (string payload) {
        try {
            var parser = new Json.Parser ();
            parser.load_from_data (payload, -1);

            unowned Json.Node root = parser.get_root ();
            unowned Json.Object root_obj = root.get_object ();
            if (!root_obj.has_member ("choices")) {
                return null;
            }

            unowned Json.Array choices = root_obj.get_array_member ("choices");
            if (choices.get_length () == 0) {
                return null;
            }

            unowned Json.Object first = choices.get_object_element (0);
            if (!first.has_member ("delta")) {
                return null;
            }

            unowned Json.Object delta = first.get_object_member ("delta");
            if (!delta.has_member ("content")) {
                return null;
            }

            return delta.get_string_member ("content");
        } catch (GLib.Error e) {
            return null;
        }
    }

    private bool extract_preview_event (string payload, out string kind, out string content) {
        kind = "";
        content = "";

        try {
            var parser = new Json.Parser ();
            parser.load_from_data (payload, -1);

            unowned Json.Node root = parser.get_root ();
            if (root == null || root.get_node_type () != Json.NodeType.OBJECT) {
                return false;
            }

            unowned Json.Object root_obj = root.get_object ();
            if (!root_obj.has_member ("x_claw_event")) {
                return false;
            }

            kind = root_obj.get_string_member ("x_claw_event");
            if (root_obj.has_member ("content")) {
                content = root_obj.get_string_member ("content");
            }
            return kind.strip ().length > 0;
        } catch (GLib.Error e) {
            return false;
        }
    }

    /* ------------------------------------------------------------------ */
    /*  Voice Bridge API (Mac Host)                                        */
    /* ------------------------------------------------------------------ */

    private const string VOICE_BRIDGE_SPEAK_URL = "http://192.168.64.1:5005/speak";
    private const string VOICE_BRIDGE_LISTEN_URL = "http://192.168.64.1:5005/listen";

    public async void speak_text (string text) {
        string clean_text = clean_markdown_for_speech (text);
        if (clean_text.length == 0) return;

        var builder = new Json.Builder ();
        builder.begin_object ();
            builder.set_member_name ("text");
            builder.add_string_value (clean_text);
        builder.end_object ();

        var generator = new Json.Generator ();
        generator.set_root (builder.get_root ());
        string payload = generator.to_data (null);

        var msg = new Soup.Message ("POST", VOICE_BRIDGE_SPEAK_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take (payload.data)
        );

        try {
            yield session.send_and_read_async (msg, GLib.Priority.DEFAULT, null);
        } catch (GLib.Error e) {
            warning ("Could not call speak on voice bridge: %s", e.message);
        }
    }

    public async string listen_voice () throws GLib.Error {
        var msg = new Soup.Message ("POST", VOICE_BRIDGE_LISTEN_URL);
        msg.set_request_body_from_bytes (
            "application/json",
            new GLib.Bytes.take ("{}".data)
        );

        var response_bytes = yield session.send_and_read_async (msg, GLib.Priority.DEFAULT, null);
        if (msg.status_code != 200) {
            throw new GLib.IOError.FAILED ("Voice bridge returned status %u".printf (msg.status_code));
        }

        unowned uint8[] raw = response_bytes.get_data ();
        var response_str = (string) raw;

        var parser = new Json.Parser ();
        parser.load_from_data (response_str, (ssize_t) response_bytes.get_size ());
        unowned Json.Node root = parser.get_root ();
        if (root == null || root.get_node_type () != Json.NodeType.OBJECT) {
            throw new GLib.IOError.FAILED ("Invalid JSON from voice bridge");
        }

        unowned Json.Object root_obj = root.get_object ();
        if (root_obj.has_member ("status") && root_obj.get_string_member ("status") == "error") {
            unowned string? err_msg = root_obj.get_string_member_with_default ("message", "Error de audio");
            throw new GLib.IOError.FAILED (err_msg);
        }

        if (!root_obj.has_member ("text")) {
            throw new GLib.IOError.FAILED ("No transcription text returned");
        }

        return root_obj.get_string_member ("text");
    }

    private string clean_markdown_for_speech (string s) {
        return s
            .replace ("**", "")
            .replace ("*", "")
            .replace ("`", "")
            .replace ("#", "")
            .replace ("_", "")
            .replace ("\r", "")
            .replace ("\n", " ")
            .strip ();
    }
}
