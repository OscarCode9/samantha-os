/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 *
 * GatewayClient — async HTTP client for the claw gateway.
 * POST /v1/chat/completions → returns assistant message content.
 *
 * The indicator keeps one session alive across follow-up questions until the
 * user explicitly starts or resets the session.
 */

public class Sam.GitHubDeviceCodeResponse : GLib.Object {
    public string device_code { get; set; }
    public string user_code { get; set; }
    public string verification_uri { get; set; }
    public int expires_in { get; set; }
    public int interval { get; set; }

    public GitHubDeviceCodeResponse (
        string device_code,
        string user_code,
        string verification_uri,
        int expires_in,
        int interval
    ) {
        this.device_code = device_code;
        this.user_code = user_code;
        this.verification_uri = verification_uri;
        this.expires_in = expires_in;
        this.interval = interval;
    }
}

public class Sam.GatewayClient : GLib.Object {

    public delegate void StreamUpdateCallback (string kind, string content);

    private const string GATEWAY_URL = "http://127.0.0.1:4389/v1/chat/completions";
    private const string GATEWAY_RUNTIME_URL = "http://127.0.0.1:4389/v1/runtime";
    private const string GATEWAY_OPENAI_CONNECT_URL = "http://127.0.0.1:4389/v1/providers/openai/connect";
    private const string GATEWAY_OPENAI_CODEX_INIT_URL = "http://127.0.0.1:4389/v1/providers/openai-codex/init";
    private const string GATEWAY_OPENAI_CODEX_EXCHANGE_URL = "http://127.0.0.1:4389/v1/providers/openai-codex/exchange";
    private const string GATEWAY_GITHUB_CONNECT_URL = "http://127.0.0.1:4389/v1/providers/github-copilot/connect";
    private const string OPENAI_CTA_PREFIX = "__GITHUB_RATE_LIMIT__::";

    private const string GITHUB_CLIENT_ID = "Iv1.b507a08c87ecfe98";
    private const string GITHUB_DEVICE_CODE_URL = "https://github.com/login/device/code";
    private const string GITHUB_ACCESS_TOKEN_URL = "https://github.com/login/oauth/access_token";

    private Soup.Session session;
    private string current_session_id;
    private string current_model = "gpt-5.4";
    private string current_provider = "github-copilot";
    private bool current_auth_configured = false;

    construct {
        session = new Soup.Session () {
            timeout = 120
        };
        start_new_session ();
    }

    public bool is_auth_configured () {
        return current_auth_configured;
    }

    public async void check_auth_status () throws GLib.Error {
        yield refresh_runtime_profile ();
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
        if (root_obj.has_member ("authConfigured")) {
            current_auth_configured = root_obj.get_boolean_member ("authConfigured");
        }
    }

    public async void connect_github_copilot_token (string token) throws GLib.Error {
        var builder = new Json.Builder ();
        builder.begin_object ();
            builder.set_member_name ("github_token");
            builder.add_string_value (token.strip ());
        builder.end_object ();

        var generator = new Json.Generator ();
        generator.set_root (builder.get_root ());
        string payload = generator.to_data (null);

        var msg = new Soup.Message ("POST", GATEWAY_GITHUB_CONNECT_URL);
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
                "No pude conectar GitHub Copilot: %s",
                body.strip ()
            );
        }

        yield refresh_runtime_profile ();
        reset_session ();
    }

    private async Json.Object post_github_form (string url, string encoded_form) throws GLib.Error {
        var message = new Soup.Message.from_encoded_form ("POST", url, encoded_form);
        message.request_headers.append ("Accept", "application/json");

        var response_bytes = yield session.send_and_read_async (message, GLib.Priority.DEFAULT, null);
        var status_code = (uint) message.status_code;
        if (status_code < 200 || status_code >= 300) {
            throw new GLib.IOError.FAILED (
                "GitHub returned HTTP %u".printf (status_code)
            );
        }

        var parser = new Json.Parser ();
        var data = response_bytes.get_data ();
        parser.load_from_data ((string) data, (ssize_t) data.length);

        var root = parser.get_root ();
        if (root == null || root.get_node_type () != Json.NodeType.OBJECT) {
            throw new GLib.IOError.FAILED ("GitHub returned an invalid JSON response.");
        }

        return root.get_object ();
    }

    public async GitHubDeviceCodeResponse request_github_device_code () throws GLib.Error {
        var response = yield post_github_form (
            GITHUB_DEVICE_CODE_URL,
            "client_id=%s&scope=%s".printf (
                Uri.escape_string (GITHUB_CLIENT_ID, null, false),
                Uri.escape_string ("read:user", null, false)
            )
        );

        return new GitHubDeviceCodeResponse (
            response.get_string_member ("device_code"),
            response.get_string_member ("user_code"),
            response.get_string_member ("verification_uri"),
            (int) response.get_int_member ("expires_in"),
            (int) response.get_int_member ("interval")
        );
    }

    public async string poll_for_github_access_token (
        string device_code,
        int interval_seconds,
        int expires_in_seconds
    ) throws GLib.Error {
        var expires_at = GLib.get_monotonic_time () + ((int64) expires_in_seconds * 1000000);
        uint interval_msec = (uint) ((interval_seconds > 0 ? interval_seconds : 1) * 1000);

        while (GLib.get_monotonic_time () < expires_at) {
            var response = yield post_github_form (
                GITHUB_ACCESS_TOKEN_URL,
                "client_id=%s&device_code=%s&grant_type=%s".printf (
                    Uri.escape_string (GITHUB_CLIENT_ID, null, false),
                    Uri.escape_string (device_code, null, false),
                    Uri.escape_string (
                        "urn:ietf:params:oauth:grant-type:device_code",
                        null,
                        false
                    )
                )
            );

            if (response.has_member ("access_token")) {
                return response.get_string_member ("access_token");
            }

            if (!response.has_member ("error")) {
                throw new GLib.IOError.FAILED ("GitHub response missing error or access_token");
            }

            var error_code = response.get_string_member ("error");
            switch (error_code) {
                case "authorization_pending":
                    yield wait_msec (interval_msec);
                    continue;
                case "slow_down":
                    yield wait_msec (interval_msec + 2000);
                    continue;
                case "access_denied":
                    throw new GLib.IOError.CANCELLED ("GitHub login was cancelled.");
                case "expired_token":
                    throw new GLib.IOError.TIMED_OUT ("GitHub device code expired.");
                default:
                    throw new GLib.IOError.FAILED (
                        "GitHub device flow failed: %s".printf (error_code)
                    );
            }
        }

        throw new GLib.IOError.TIMED_OUT ("GitHub device code expired.");
    }

    private async void wait_msec (uint wait_time) {
        SourceFunc callback = wait_msec.callback;

        Timeout.add (wait_time, () => {
            callback ();
            return Source.REMOVE;
        });

        yield;
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

            var parser = new Json.Parser ();
            try {
                parser.load_from_data (err_body, -1);
                unowned Json.Node? root = parser.get_root ();
                if (root != null && root.get_node_type () == Json.NodeType.OBJECT) {
                    unowned Json.Object root_obj = root.get_object ();
                    if (root_obj.has_member ("error")) {
                        unowned Json.Object error_obj = root_obj.get_object_member ("error");
                        if (error_obj.has_member ("type") && error_obj.get_string_member ("type") == "usage_limit_reached") {
                            int64 resets_in = 0;
                            if (error_obj.has_member ("resets_in_seconds")) {
                                resets_in = error_obj.get_int_member ("resets_in_seconds");
                            }
                            string plan = "Plus";
                            if (error_obj.has_member ("plan_type")) {
                                plan = error_obj.get_string_member ("plan_type");
                                if (plan == "plus") {
                                    plan = "Plus";
                                } else if (plan == "free") {
                                    plan = "Free";
                                }
                            }
                            
                            int64 hours = resets_in / 3600;
                            int64 minutes = (resets_in % 3600) / 60;
                            
                            string reset_time = "";
                            if (hours > 0) {
                                reset_time += "%lld hora%s".printf (hours, hours > 1 ? "s" : "");
                            }
                            if (minutes > 0) {
                                if (hours > 0) {
                                    reset_time += " y ";
                                }
                                reset_time += "%lld minuto%s".printf (minutes, minutes > 1 ? "s" : "");
                            }
                            if (reset_time == "") {
                                reset_time = "unos segundos";
                            }
                            
                            return "Límite de uso alcanzado para tu plan %s. Se restablecerá en %s.".printf (plan, reset_time);
                        }
                    }
                }
            } catch (GLib.Error e) {
                // fall back to raw err_body
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
