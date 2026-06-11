/*
 * SPDX-License-Identifier: GPL-3.0-or-later
 *
 * panel-sam — Wingpanel indicator for SAM AI assistant
 *
 * Display widget (bar): search icon + "Ask Samantha" label.
 * Popover: coral-orange theme (#C44C35), entry auto-focused on open.
 */

public class Sam.Indicator : Wingpanel.Indicator {

    private const string CHAT_OPEN_SOUND_FILE = "chat-open.mp3";
    private const bool ENABLE_VOICE = false;

    private delegate void SessionAction ();

    /* -- Panel bar display widget -- */
    private Gtk.Box display_box;

    /* -- Popover widgets -- */
    private Gtk.Box     popover_box;
    private Gtk.Entry   query_entry;
    private Gtk.Button  send_button;
    private Gtk.Button  reset_session_button;
    private Gtk.Button? mic_button = null;
    private Gtk.Spinner popover_spinner;
    private Gtk.Box     activity_box;
    private Gtk.Label   activity_title_label;
    private Gtk.Label   activity_status_label;
    private Gtk.Label   activity_preview_label;
    private Gtk.ScrolledWindow result_scroll;
    private Gtk.Box            result_container;
    private bool               request_in_flight = false;
    private uint               activity_animation_id = 0;
    private int                activity_animation_frame = 0;
    private string             activity_phase_base = "Pensando y preparando respuesta";

    /* -- Gateway client -- */
    private GatewayClient gateway;

    /* CSS is loaded from Build.PKGDATADIR/styles/sam.css at runtime.
     * See panel-sam/data/styles/sam.css for all widget styles. */

    /* ------------------------------------------------------------------ */
    /*  Constructor                                                         */
    /* ------------------------------------------------------------------ */

    public Indicator () {
        Object (code_name: "sam");
    }

    construct {
        visible = true;
        load_css ();
        gateway = new GatewayClient ();
    }

    private void load_css () {
        var provider = new Gtk.CssProvider ();
        var css_path = GLib.Path.build_filename (Build.PKGDATADIR, "styles", "sam.css");
        try {
            provider.load_from_path (css_path);
        } catch (Error e) {
            GLib.warning ("panel-sam: CSS load error (%s): %s", css_path, e.message);
        }
        unowned Gdk.Screen? screen = Gdk.Screen.get_default ();
        if (screen != null) {
            Gtk.StyleContext.add_provider_for_screen (
                screen,
                provider,
                Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
            );
        }
    }

    private void play_chat_open_sound () {
        var sound_path = GLib.Path.build_filename (Build.PKGDATADIR, "sounds", CHAT_OPEN_SOUND_FILE);
        if (!GLib.FileUtils.test (sound_path, GLib.FileTest.EXISTS)) {
            return;
        }

        string[]? argv = build_chat_open_sound_argv (sound_path);
        if (argv == null) {
            GLib.warning ("panel-sam: no audio player found for %s", sound_path);
            return;
        }

        try {
            new Subprocess.newv (argv, SubprocessFlags.NONE);
        } catch (Error e) {
            GLib.warning ("panel-sam: could not play open sound: %s", e.message);
        }
    }

    private string[]? build_chat_open_sound_argv (string sound_path) {
        string? gst_play = Environment.find_program_in_path ("gst-play-1.0");
        if (gst_play != null && gst_play.strip () != "") {
            return { gst_play, "-q", "--no-interactive", sound_path, null };
        }

        string? paplay = Environment.find_program_in_path ("paplay");
        if (paplay != null && paplay.strip () != "") {
            return { paplay, sound_path, null };
        }

        string? canberra = Environment.find_program_in_path ("canberra-gtk-play");
        if (canberra != null && canberra.strip () != "") {
            return { canberra, "-f", sound_path, null };
        }

        return null;
    }

    /* ------------------------------------------------------------------ */
    /*  Wingpanel.Indicator API                                             */
    /* ------------------------------------------------------------------ */

    public override Gtk.Widget get_display_widget () {
        if (display_box == null) {
            var icon = new Gtk.Image.from_icon_name (
                "system-search-symbolic",
                Gtk.IconSize.SMALL_TOOLBAR
            );

            var lbl = new Gtk.Label ("Ask Samantha") {
                margin_start = 5
            };

            display_box = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 0) {
                margin_start = 6,
                margin_end   = 6,
                tooltip_text = "SAM \u2014 pregunta algo"
            };
            display_box.pack_start (icon, false, false, 0);
            display_box.pack_start (lbl,  false, false, 0);
            display_box.show_all ();
        }

        return display_box;
    }

    public override Gtk.Widget? get_widget () {
        if (popover_box == null) {
            build_popover ();
        }

        return popover_box;
    }

    public override void opened () {
        play_chat_open_sound ();
        check_auth_and_sync ();
    }

    private void check_auth_and_sync () {
        set_controls_sensitive (false);
        query_entry.placeholder_text = "Verificando conexión…";
        gateway.check_auth_status.begin ((obj, res) => {
            bool auth_configured = false;
            try {
                gateway.check_auth_status.end (res);
                auth_configured = gateway.is_auth_configured ();
            } catch (GLib.Error e) {
                warning ("Could not check auth status: %s", e.message);
            }

            GLib.Idle.add (() => {
                if (auth_configured) {
                    set_controls_sensitive (true);
                    query_entry.placeholder_text = "Pregunta algo…";
                    sync_popover_state ();
                    if (query_entry != null) {
                        query_entry.grab_focus ();
                    }
                } else {
                    query_entry.placeholder_text = "Conecta un proveedor de IA…";
                    set_controls_sensitive (false);
                    show_provider_connect_screen ();
                }
                return GLib.Source.REMOVE;
            });
        });
    }

    private void show_provider_connect_screen () {
        clear_results ();

        var title = new Gtk.Label ("Conectar un proveedor de IA") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        title.get_style_context ().add_class ("sam-provider-title");

        var copy = new Gtk.Label (
            "Para comenzar a usar Samantha, conecta tu cuenta de GitHub Copilot o ingresa tus credenciales de OpenAI."
        ) {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        copy.get_style_context ().add_class ("sam-provider-copy");

        var status_label = new Gtk.Label ("") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44,
            visible = false
        };
        status_label.get_style_context ().add_class ("sam-provider-status");

        // GitHub Copilot section
        var github_btn = new Gtk.Button.with_label ("Conectar con GitHub Copilot") {
            halign = Gtk.Align.START
        };
        github_btn.get_style_context ().add_class ("sam-provider-btn");

        // GitHub flow UI elements
        var github_code_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 6) {
            visible = false
        };
        var github_url_hint = new Gtk.Label ("Ingresa este código en github.com/login/device:") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        github_url_hint.get_style_context ().add_class ("sam-provider-copy");
        
        var github_code_lbl = new Gtk.Label ("") {
            halign = Gtk.Align.CENTER,
            xalign = 0.5f,
            wrap = false,
            selectable = true
        };
        github_code_lbl.get_style_context ().add_class ("sam-provider-title");

        var github_open_btn = new Gtk.Button.with_label ("Abrir GitHub en el navegador") {
            halign = Gtk.Align.START
        };
        github_open_btn.get_style_context ().add_class ("sam-provider-btn");

        github_code_box.pack_start (github_url_hint, false, false, 0);
        github_code_box.pack_start (github_code_lbl, false, false, 8);
        github_code_box.pack_start (github_open_btn, false, false, 0);

        // OpenAI/ChatGPT section
        var openai_btn = new Gtk.Button.with_label ("Conectar con OpenAI / ChatGPT") {
            halign = Gtk.Align.START
        };
        openai_btn.get_style_context ().add_class ("sam-provider-btn");

        // We can reuse the sub-revealers or nested forms for OpenAI connection:
        // 1. ChatGPT subscription
        var subscription_hint = new Gtk.Label (
            "Abriremos ChatGPT en tu navegador. Cuando termine, copia la URL completa de redirección y pégala aquí."
        ) {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        subscription_hint.get_style_context ().add_class ("sam-provider-copy");

        var redirect_url_entry = new Gtk.Entry () {
            placeholder_text = "http://localhost:1455/auth/callback?code=...",
            hexpand = true,
            has_frame = false
        };
        redirect_url_entry.get_style_context ().add_class ("sam-provider-entry");

        var finish_subscription_btn = new Gtk.Button.with_label ("Guardar y conectar") {
            halign = Gtk.Align.START
        };
        finish_subscription_btn.get_style_context ().add_class ("sam-provider-btn");

        var openai_sub_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
        openai_sub_row.pack_start (redirect_url_entry, true, true, 0);
        openai_sub_row.pack_start (finish_subscription_btn, false, false, 0);

        var openai_sub_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8);
        openai_sub_box.pack_start (subscription_hint, false, false, 0);
        openai_sub_box.pack_start (openai_sub_row, false, false, 0);

        var openai_sub_revealer = new Gtk.Revealer () {
            transition_type = Gtk.RevealerTransitionType.SLIDE_DOWN,
            reveal_child = false
        };
        openai_sub_revealer.add (openai_sub_box);

        // 2. API Key option
        var api_key_reveal_btn = new Gtk.Button.with_label ("Usar API key de OpenAI en su lugar") {
            halign = Gtk.Align.START
        };
        api_key_reveal_btn.get_style_context ().add_class ("sam-provider-btn");

        var api_key_entry = new Gtk.Entry () {
            placeholder_text = "sk-...",
            hexpand = true,
            has_frame = false,
            visibility = false
        };
        api_key_entry.get_style_context ().add_class ("sam-provider-entry");

        var connect_api_key_btn = new Gtk.Button.with_label ("Guardar API key") {
            halign = Gtk.Align.START
        };
        connect_api_key_btn.get_style_context ().add_class ("sam-provider-btn");

        var openai_api_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
        openai_api_row.pack_start (api_key_entry, true, true, 0);
        openai_api_row.pack_start (connect_api_key_btn, false, false, 0);

        var openai_api_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8);
        openai_api_box.pack_start (openai_api_row, false, false, 0);

        var openai_api_revealer = new Gtk.Revealer () {
            transition_type = Gtk.RevealerTransitionType.SLIDE_DOWN,
            reveal_child = false
        };
        openai_api_revealer.add (openai_api_box);

        var openai_options_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8) {
            visible = false
        };
        var opt1_lbl = new Gtk.Label ("Opción 1: Cuenta ChatGPT Plus") {
            halign = Gtk.Align.START,
            xalign = 0.0f
        };
        opt1_lbl.get_style_context ().add_class ("sam-provider-title");
        var chatgpt_auth_btn = new Gtk.Button.with_label ("Conectar con ChatGPT") {
            halign = Gtk.Align.START
        };
        chatgpt_auth_btn.get_style_context ().add_class ("sam-provider-btn");
        openai_options_box.pack_start (opt1_lbl, false, false, 0);
        openai_options_box.pack_start (chatgpt_auth_btn, false, false, 0);
        openai_options_box.pack_start (openai_sub_revealer, false, false, 0);
        var opt2_lbl = new Gtk.Label ("Opción 2: API de OpenAI") {
            halign = Gtk.Align.START,
            xalign = 0.0f
        };
        opt2_lbl.get_style_context ().add_class ("sam-provider-title");
        openai_options_box.pack_start (opt2_lbl, false, false, 4);
        openai_options_box.pack_start (api_key_reveal_btn, false, false, 0);
        openai_options_box.pack_start (openai_api_revealer, false, false, 0);

        // Wire events!
        string verification_uri = "";
        string auth_url = "";

        github_btn.clicked.connect (() => {
            github_btn.sensitive = false;
            openai_btn.sensitive = false;
            openai_options_box.visible = false;
            status_label.label = "Conectando con GitHub…";
            status_label.visible = true;

            gateway.request_github_device_code.begin ((obj, res) => {
                string error_message = "";
                GitHubDeviceCodeResponse device_resp = null;
                try {
                    device_resp = gateway.request_github_device_code.end (res);
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    if (device_resp == null) {
                        github_btn.sensitive = true;
                        openai_btn.sensitive = true;
                        status_label.label = "Error al contactar a GitHub: " + error_message;
                        status_label.visible = true;
                        return GLib.Source.REMOVE;
                    }

                    verification_uri = device_resp.verification_uri;
                    github_code_lbl.set_markup ("<span size=\"xx-large\" weight=\"bold\">%s</span>".printf (device_resp.user_code));
                    github_code_box.visible = true;
                    status_label.label = "Autoriza la solicitud en tu navegador y regresa.";
                    status_label.visible = true;

                    try {
                        Gtk.show_uri_on_window (null, verification_uri, Gdk.CURRENT_TIME);
                    } catch (GLib.Error e) {
                        warning ("Could not launch browser: %s", e.message);
                    }

                    gateway.poll_for_github_access_token.begin (
                        device_resp.device_code,
                        device_resp.interval,
                        device_resp.expires_in,
                        (obj2, res2) => {
                            string token = "";
                            string poll_error = "";
                            try {
                                token = gateway.poll_for_github_access_token.end (res2);
                            } catch (GLib.Error e) {
                                poll_error = e.message;
                            }

                            GLib.Idle.add (() => {
                                if (token == "") {
                                    github_btn.sensitive = true;
                                    openai_btn.sensitive = true;
                                    github_code_box.visible = false;
                                    status_label.label = "Inicio de sesión cancelado o expirado: " + poll_error;
                                    status_label.visible = true;
                                    return GLib.Source.REMOVE;
                                }

                                status_label.label = "Conectando token con Samantha…";
                                gateway.connect_github_copilot_token.begin (token, (obj3, res3) => {
                                    string conn_error = "";
                                    bool success = false;
                                    try {
                                        gateway.connect_github_copilot_token.end (res3);
                                        success = true;
                                    } catch (GLib.Error e) {
                                        conn_error = e.message;
                                    }

                                    GLib.Idle.add (() => {
                                        if (success) {
                                            status_label.label = "Conectado con éxito. Reiniciando…";
                                            GLib.Timeout.add (1500, () => {
                                                check_auth_and_sync ();
                                                return GLib.Source.REMOVE;
                                            });
                                        } else {
                                            github_btn.sensitive = true;
                                            openai_btn.sensitive = true;
                                            github_code_box.visible = false;
                                            status_label.label = "Error al guardar token: " + conn_error;
                                        }
                                        return GLib.Source.REMOVE;
                                    });
                                });
                                return GLib.Source.REMOVE;
                            });
                        }
                    );

                    return GLib.Source.REMOVE;
                });
            });
        });

        github_open_btn.clicked.connect (() => {
            if (verification_uri != "") {
                try {
                    Gtk.show_uri_on_window (null, verification_uri, Gdk.CURRENT_TIME);
                } catch (GLib.Error e) {
                    warning ("Could not launch browser: %s", e.message);
                }
            }
        });

        openai_btn.clicked.connect (() => {
            openai_options_box.visible = !openai_options_box.visible;
            github_code_box.visible = false;
            github_btn.sensitive = true;
            status_label.visible = false;
        });

        chatgpt_auth_btn.clicked.connect (() => {
            chatgpt_auth_btn.sensitive = false;
            status_label.label = "Abriendo conexión con ChatGPT…";
            status_label.visible = true;

            gateway.begin_openai_codex_subscription.begin ((obj, res) => {
                string error_message = "";
                string started_auth_url = "";
                try {
                    started_auth_url = gateway.begin_openai_codex_subscription.end (res);
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    chatgpt_auth_btn.sensitive = true;
                    if (started_auth_url == "") {
                        status_label.label = "Error: " + error_message;
                        return GLib.Source.REMOVE;
                    }

                    auth_url = started_auth_url;
                    openai_sub_revealer.reveal_child = true;
                    status_label.label = "Inicia sesión en ChatGPT y pega la URL final.";
                    try {
                        Gtk.show_uri_on_window (null, auth_url, Gdk.CURRENT_TIME);
                    } catch (GLib.Error e) {
                        warning ("Could not open browser: %s", e.message);
                    }
                    return GLib.Source.REMOVE;
                });
            });
        });

        finish_subscription_btn.clicked.connect (() => {
            var raw_url = redirect_url_entry.text.strip ();
            if (raw_url.length == 0) {
                status_label.label = "Pega la URL completa para continuar.";
                status_label.visible = true;
                return;
            }

            finish_subscription_btn.sensitive = false;
            status_label.label = "Guardando suscripción ChatGPT…";
            gateway.finish_openai_codex_subscription.begin (raw_url, (obj, res) => {
                string error_message = "";
                bool success = false;
                try {
                    gateway.finish_openai_codex_subscription.end (res);
                    success = true;
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    finish_subscription_btn.sensitive = true;
                    if (success) {
                        status_label.label = "ChatGPT conectado con éxito. Reiniciando…";
                        GLib.Timeout.add (1500, () => {
                            check_auth_and_sync ();
                            return GLib.Source.REMOVE;
                        });
                    } else {
                        status_label.label = "Error: " + error_message;
                    }
                    return GLib.Source.REMOVE;
                });
            });
        });

        api_key_reveal_btn.clicked.connect (() => {
            openai_api_revealer.reveal_child = !openai_api_revealer.reveal_child;
        });

        connect_api_key_btn.clicked.connect (() => {
            var api_key = api_key_entry.text.strip ();
            if (api_key.length == 0) {
                status_label.label = "Pega tu API key de OpenAI.";
                status_label.visible = true;
                return;
            }

            connect_api_key_btn.sensitive = false;
            status_label.label = "Guardando API key…";
            gateway.connect_openai_codex_api_key.begin (api_key, (obj, res) => {
                string error_message = "";
                bool success = false;
                try {
                    gateway.connect_openai_codex_api_key.end (res);
                    success = true;
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    connect_api_key_btn.sensitive = true;
                    if (success) {
                        status_label.label = "API key conectada con éxito. Reiniciando…";
                        GLib.Timeout.add (1500, () => {
                            check_auth_and_sync ();
                            return GLib.Source.REMOVE;
                        });
                    } else {
                        status_label.label = "Error: " + error_message;
                    }
                    return GLib.Source.REMOVE;
                });
            });
        });

        var card = new Gtk.Box (Gtk.Orientation.VERTICAL, 10);
        card.get_style_context ().add_class ("sam-provider-card");
        card.pack_start (title, false, false, 0);
        card.pack_start (copy, false, false, 0);
        card.pack_start (github_btn, false, false, 0);
        card.pack_start (github_code_box, false, false, 0);
        card.pack_start (openai_btn, false, false, 0);
        card.pack_start (openai_options_box, false, false, 0);
        card.pack_start (status_label, false, false, 0);

        result_container.add (card);
        result_container.show_all ();
    }

    public override void closed () { }

    /* ------------------------------------------------------------------ */
    /*  Popover                                                             */
    /* ------------------------------------------------------------------ */

    private void build_popover () {
        /* Entry — has_frame = false kills the GTK3 border/shadow entirely */
        query_entry = new Gtk.Entry () {
            placeholder_text = "Pregunta algo\u2026",
            hexpand       = true,
            has_frame     = false,
            margin_top    = 10,
            margin_bottom = 10,
            margin_start  = 12,
            margin_end    = 8
        };
        query_entry.get_style_context ().add_class ("sam-entry");
        query_entry.activate.connect (on_query_submitted);

        /* Send button */
        send_button = new Gtk.Button.with_label ("\u2191") {
            valign     = Gtk.Align.CENTER,
            margin_end = 10
        };
        send_button.get_style_context ().add_class ("sam-send");
        send_button.clicked.connect (on_query_submitted);

        reset_session_button = create_session_button (
            "edit-clear-symbolic",
            "Reiniciar sesi\u00f3n",
            on_reset_session_requested
        );

        if (ENABLE_VOICE) {
            mic_button = create_session_button (
                "audio-input-microphone-symbolic",
                "Hablar con Samantha",
                on_mic_button_clicked
            );
        }

        var session_box = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 6) {
            margin_end = 8,
            valign = Gtk.Align.CENTER
        };
        if (mic_button != null) {
            session_box.pack_start (mic_button, false, false, 0);
        }
        session_box.pack_start (reset_session_button, false, false, 0);

        /* Input row */
        var input_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 0);
        input_row.pack_start (query_entry, true,  true,  0);
        input_row.pack_start (session_box, false, false, 0);
        input_row.pack_start (send_button, false, false, 0);

        /* Divider */
        var divider = new Gtk.Separator (Gtk.Orientation.HORIZONTAL);
        divider.get_style_context ().add_class ("sam-divider");

        /* Spinner */
        popover_spinner = new Gtk.Spinner () {
            halign        = Gtk.Align.START,
            visible       = false,
            no_show_all   = true
        };
        popover_spinner.get_style_context ().add_class ("sam-spinner");

        activity_title_label = new Gtk.Label ("El agente est\u00e1 trabajando\u2026") {
            halign = Gtk.Align.START,
            xalign = 0.0f
        };
        activity_title_label.get_style_context ().add_class ("sam-activity-title");

        activity_status_label = new Gtk.Label ("Pensando y preparando respuesta\u2026") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 42
        };
        activity_status_label.get_style_context ().add_class ("sam-activity-status");

        activity_preview_label = new Gtk.Label ("") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 42,
            selectable = true,
            visible = false,
            no_show_all = true
        };
        activity_preview_label.get_style_context ().add_class ("sam-activity-preview");

        var activity_text_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 4);
        activity_text_box.pack_start (activity_title_label, false, false, 0);
        activity_text_box.pack_start (activity_status_label, false, false, 0);

        var activity_header = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 10);
        activity_header.pack_start (popover_spinner, false, false, 0);
        activity_header.pack_start (activity_text_box, true, true, 0);

        activity_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 10) {
            visible = false,
            no_show_all = true
        };
        activity_box.get_style_context ().add_class ("sam-activity-box");
        activity_box.pack_start (activity_header, false, false, 0);
        activity_box.pack_start (activity_preview_label, false, false, 0);

        /* Result area — scrollable container with mixed text + file widgets */
        result_container = new Gtk.Box (Gtk.Orientation.VERTICAL, 4) {
            margin_top    = 8,
            margin_bottom = 12,
            margin_start  = 12,
            margin_end    = 12
        };
        show_placeholder_result ("Escribe una pregunta y presiona Enter\u2026");

        result_scroll = new Gtk.ScrolledWindow (null, null) {
            hscrollbar_policy        = Gtk.PolicyType.NEVER,
            vscrollbar_policy        = Gtk.PolicyType.AUTOMATIC,
            max_content_height       = 800,
            propagate_natural_height = true
        };
        result_scroll.add (result_container);

        /* Assemble */
        popover_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 0) {
            width_request = 480
        };
        popover_box.get_style_context ().add_class ("sam-popover");
        popover_box.add (input_row);
        popover_box.add (divider);
        popover_box.add (activity_box);
        popover_box.add (result_scroll);
        popover_box.show_all ();
        hide_activity_view ();
        sync_popover_state ();
    }

    /* ------------------------------------------------------------------ */
    /*  Query handler (Phase 3: replace stub with GatewayClient.chat())    */
    /* ------------------------------------------------------------------ */

    private void on_query_submitted () {
        var query = query_entry.text.strip ();
        if (query.length == 0) {
            return;
        }

        query_entry.text = "";
        submit_query (query);
    }

    private void submit_query (string query) {
        set_controls_sensitive (false);
        request_in_flight = true;
        show_activity_view ("Pensando y preparando respuesta\u2026");

        gateway.chat_stream.begin (query, (kind, content) => {
            GLib.Idle.add (() => {
                update_activity_view (kind, content);
                return GLib.Source.REMOVE;
            });
        }, (obj, res) => {
            string answer = "";
            string offer_detail = "";
            bool should_offer_openai = false;
            try {
                answer = gateway.chat_stream.end (res);
            } catch (GLib.Error e) {
                if (gateway.should_offer_openai_codex (e.message)) {
                    should_offer_openai = true;
                    offer_detail = gateway.normalize_error_message (e.message);
                } else {
                    answer = "\u26a0 Error: " + e.message;
                }
            }

            GLib.Idle.add (() => {
                request_in_flight = false;
                hide_activity_view ();
                if (should_offer_openai) {
                    show_openai_codex_offer (query, offer_detail);
                } else {
                    populate_results (answer);
                    gateway.speak_text.begin (answer);
                    query_entry.grab_focus ();
                }
                result_scroll.show ();
                set_controls_sensitive (true);
                return GLib.Source.REMOVE;
            });
        });
    }

    private void show_openai_codex_offer (string query, string detail) {
        clear_results ();

        var title = new Gtk.Label ("GitHub alcanzó su límite de sesión.") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        title.get_style_context ().add_class ("sam-provider-title");

        var copy = new Gtk.Label (
            "Puedes seguir esta conversación conectando tu suscripción de ChatGPT a OpenAI Codex. Si prefieres, también puedes usar una API key como alternativa."
        ) {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        copy.get_style_context ().add_class ("sam-provider-copy");

        var detail_label = new Gtk.Label (detail) {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            selectable = true,
            max_width_chars = 44
        };
        detail_label.get_style_context ().add_class ("sam-provider-status");

        var subscription_button = new Gtk.Button.with_label ("Conectar con ChatGPT");
        subscription_button.get_style_context ().add_class ("sam-provider-btn");
        subscription_button.halign = Gtk.Align.START;

        var subscription_hint = new Gtk.Label (
            "Abriremos ChatGPT en tu navegador. Cuando termine, puede aparecer un error en localhost: es normal. Copia esa URL completa y pégala aquí."
        ) {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44
        };
        subscription_hint.get_style_context ().add_class ("sam-provider-copy");

        var reopen_browser_button = new Gtk.Button.with_label ("Abrir ChatGPT de nuevo");
        reopen_browser_button.get_style_context ().add_class ("sam-provider-btn");
        reopen_browser_button.halign = Gtk.Align.START;
        reopen_browser_button.visible = false;

        var redirect_url_entry = new Gtk.Entry () {
            placeholder_text = "http://localhost:1455/auth/callback?code=...&state=...",
            hexpand = true,
            has_frame = false
        };
        redirect_url_entry.get_style_context ().add_class ("sam-provider-entry");

        var finish_subscription_button = new Gtk.Button.with_label ("Pegar URL y reintentar");
        finish_subscription_button.get_style_context ().add_class ("sam-provider-btn");

        var status_label = new Gtk.Label ("") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44,
            visible = false
        };
        status_label.get_style_context ().add_class ("sam-provider-status");

        string auth_url = "";

        var subscription_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
        subscription_row.pack_start (redirect_url_entry, true, true, 0);
        subscription_row.pack_start (finish_subscription_button, false, false, 0);

        var subscription_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8);
        subscription_box.pack_start (subscription_hint, false, false, 0);
        subscription_box.pack_start (reopen_browser_button, false, false, 0);
        subscription_box.pack_start (subscription_row, false, false, 0);
        subscription_box.pack_start (status_label, false, false, 0);

        var subscription_revealer = new Gtk.Revealer () {
            transition_type = Gtk.RevealerTransitionType.SLIDE_DOWN,
            reveal_child = false
        };
        subscription_revealer.add (subscription_box);

        reopen_browser_button.clicked.connect (() => {
            if (auth_url.strip ().length == 0) {
                return;
            }

            try {
                Gtk.show_uri_on_window (null, auth_url, Gdk.CURRENT_TIME);
            } catch (GLib.Error e) {
                status_label.label = "No pude abrir el navegador automáticamente. Usa este mismo botón para intentarlo otra vez.";
                status_label.visible = true;
            }
        });

        subscription_button.clicked.connect (() => {
            status_label.label = "Abriendo la conexión con ChatGPT…";
            status_label.visible = true;
            subscription_button.sensitive = false;
            redirect_url_entry.sensitive = false;
            finish_subscription_button.sensitive = false;
            set_controls_sensitive (false);

            gateway.begin_openai_codex_subscription.begin ((obj, res) => {
                string error_message = "";
                bool started = false;
                string started_auth_url = "";
                try {
                    started_auth_url = gateway.begin_openai_codex_subscription.end (res);
                    started = true;
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    subscription_button.sensitive = true;
                    redirect_url_entry.sensitive = true;
                    finish_subscription_button.sensitive = true;
                    set_controls_sensitive (true);

                    if (!started) {
                        status_label.label = error_message;
                        status_label.visible = true;
                        return GLib.Source.REMOVE;
                    }

                    auth_url = started_auth_url;
                    subscription_revealer.reveal_child = true;
                    reopen_browser_button.visible = true;
                    redirect_url_entry.grab_focus ();
                    status_label.label = "Inicia sesión en ChatGPT y luego pega aquí la URL final que termine en localhost.";
                    status_label.visible = true;
                    try {
                        Gtk.show_uri_on_window (null, auth_url, Gdk.CURRENT_TIME);
                    } catch (GLib.Error e) {
                        status_label.label = "No pude abrir el navegador automáticamente. Usa “Abrir ChatGPT de nuevo” para continuar.";
                        status_label.visible = true;
                    }
                    return GLib.Source.REMOVE;
                });
            });
        });

        finish_subscription_button.clicked.connect (() => {
            var raw_url = redirect_url_entry.text.strip ();
            if (raw_url.length == 0) {
                status_label.label = "Pega la URL completa de redirección para continuar.";
                status_label.visible = true;
                redirect_url_entry.grab_focus ();
                return;
            }

            status_label.label = "Conectando tu suscripción de ChatGPT…";
            status_label.visible = true;
            subscription_button.sensitive = false;
            reopen_browser_button.sensitive = false;
            finish_subscription_button.sensitive = false;
            redirect_url_entry.sensitive = false;
            set_controls_sensitive (false);

            gateway.finish_openai_codex_subscription.begin (raw_url, (obj, res) => {
                string error_message = "";
                bool connected = false;
                try {
                    gateway.finish_openai_codex_subscription.end (res);
                    connected = true;
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    subscription_button.sensitive = true;
                    reopen_browser_button.sensitive = true;
                    finish_subscription_button.sensitive = true;
                    redirect_url_entry.sensitive = true;
                    set_controls_sensitive (true);

                    if (connected) {
                        show_placeholder_result ("ChatGPT conectado. Reintentando tu pregunta…");
                        submit_query (query);
                        return GLib.Source.REMOVE;
                    }

                    status_label.label = error_message;
                    status_label.visible = true;
                    return GLib.Source.REMOVE;
                });
            });
        });

        redirect_url_entry.activate.connect (() => {
            finish_subscription_button.clicked ();
        });

        var api_key_reveal_button = new Gtk.Button.with_label ("Usar API key en su lugar");
        api_key_reveal_button.get_style_context ().add_class ("sam-provider-btn");
        api_key_reveal_button.halign = Gtk.Align.START;

        var api_key_entry = new Gtk.Entry () {
            placeholder_text = "sk-...",
            hexpand = true,
            has_frame = false,
            visibility = false
        };
        api_key_entry.get_style_context ().add_class ("sam-provider-entry");

        var connect_button = new Gtk.Button.with_label ("Guardar API key y reintentar");
        connect_button.get_style_context ().add_class ("sam-provider-btn");

        var api_status_label = new Gtk.Label ("") {
            halign = Gtk.Align.START,
            xalign = 0.0f,
            wrap = true,
            max_width_chars = 44,
            visible = false
        };
        api_status_label.get_style_context ().add_class ("sam-provider-status");

        var api_form_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
        api_form_row.pack_start (api_key_entry, true, true, 0);
        api_form_row.pack_start (connect_button, false, false, 0);

        var api_form_box = new Gtk.Box (Gtk.Orientation.VERTICAL, 8);
        api_form_box.pack_start (api_form_row, false, false, 0);
        api_form_box.pack_start (api_status_label, false, false, 0);

        var api_form_revealer = new Gtk.Revealer () {
            transition_type = Gtk.RevealerTransitionType.SLIDE_DOWN,
            reveal_child = false
        };
        api_form_revealer.add (api_form_box);

        api_key_reveal_button.clicked.connect (() => {
            api_form_revealer.reveal_child = !api_form_revealer.reveal_child;
            if (api_form_revealer.reveal_child) {
                api_key_entry.grab_focus ();
            }
        });

        connect_button.clicked.connect (() => {
            var api_key = api_key_entry.text.strip ();
            if (api_key.length == 0) {
                api_status_label.label = "Pega tu API key de OpenAI para continuar.";
                api_status_label.visible = true;
                api_key_entry.grab_focus ();
                return;
            }

            api_status_label.label = "Conectando OpenAI Codex…";
            api_status_label.visible = true;
            api_key_reveal_button.sensitive = false;
            connect_button.sensitive = false;
            api_key_entry.sensitive = false;
            set_controls_sensitive (false);

            gateway.connect_openai_codex_api_key.begin (api_key, (obj, res) => {
                string error_message = "";
                bool connected = false;
                try {
                    gateway.connect_openai_codex_api_key.end (res);
                    connected = true;
                } catch (GLib.Error e) {
                    error_message = e.message;
                }

                GLib.Idle.add (() => {
                    api_key_reveal_button.sensitive = true;
                    connect_button.sensitive = true;
                    api_key_entry.sensitive = true;
                    set_controls_sensitive (true);

                    if (connected) {
                        show_placeholder_result ("OpenAI Codex conectado. Reintentando tu pregunta…");
                        submit_query (query);
                        return GLib.Source.REMOVE;
                    }

                    api_status_label.label = error_message;
                    api_status_label.visible = true;
                    return GLib.Source.REMOVE;
                });
            });
        });

        api_key_entry.activate.connect (() => {
            connect_button.clicked ();
        });

        var card = new Gtk.Box (Gtk.Orientation.VERTICAL, 10);
        card.get_style_context ().add_class ("sam-provider-card");
        card.pack_start (title, false, false, 0);
        card.pack_start (copy, false, false, 0);
        card.pack_start (detail_label, false, false, 0);
        card.pack_start (subscription_button, false, false, 0);
        card.pack_start (subscription_revealer, false, false, 0);
        card.pack_start (api_key_reveal_button, false, false, 0);
        card.pack_start (api_form_revealer, false, false, 0);

        result_container.add (card);
        result_container.show_all ();
    }

    private Gtk.Button create_session_button (string icon_name, string tooltip, owned SessionAction action) {
        var icon = new Gtk.Image.from_icon_name (icon_name, Gtk.IconSize.MENU);
        var button = new Gtk.Button () {
            relief = Gtk.ReliefStyle.NONE,
            tooltip_text = tooltip,
            valign = Gtk.Align.CENTER
        };
        button.add (icon);
        button.get_style_context ().add_class ("sam-session-btn");
        button.clicked.connect (() => {
            action ();
        });
        return button;
    }

    private void show_activity_view (string phase) {
        set_activity_phase (phase);
        activity_preview_label.label = "";
        activity_preview_label.hide ();
        popover_spinner.show ();
        popover_spinner.start ();
        start_activity_animation ();
        activity_box.show ();
        result_scroll.hide ();
    }

    private void update_activity_view (string kind, string content) {
        if (kind == "phase") {
            set_activity_phase (content);
            return;
        }

        if (kind == "tool" || kind == "error") {
            if (kind == "tool") {
                set_activity_phase ("Ejecutando tools");
            }
            activity_preview_label.label = content;
            if (content.strip ().length > 0) {
                activity_preview_label.show ();
            } else {
                activity_preview_label.hide ();
            }
            return;
        }

        if (kind == "preview") {
            activity_preview_label.label = content;
            if (content.strip ().length > 0) {
                activity_preview_label.show ();
            } else {
                activity_preview_label.hide ();
            }
        }
    }

    private void hide_activity_view () {
        stop_activity_animation ();
        popover_spinner.stop ();
        popover_spinner.hide ();
        activity_phase_base = "Pensando y preparando respuesta";
        activity_status_label.label = "Pensando y preparando respuesta\u2026";
        activity_preview_label.label = "";
        activity_preview_label.hide ();
        activity_box.hide ();
    }

    private void sync_popover_state () {
        if (request_in_flight) {
            if (activity_animation_id == 0) {
                start_activity_animation ();
            }
            activity_box.show ();
            popover_spinner.show ();
            result_scroll.hide ();
            if (activity_preview_label.label.strip ().length > 0) {
                activity_preview_label.show ();
            } else {
                activity_preview_label.hide ();
            }
            return;
        }

        hide_activity_view ();
        result_scroll.show ();
    }

    private void set_activity_phase (string phase) {
        activity_phase_base = normalize_activity_phase (phase);
        update_activity_status_animation ();
    }

    private string normalize_activity_phase (string phase) {
        var normalized = phase.strip ();

        while (normalized.has_suffix ("\u2026")) {
            normalized = normalized.substring (0, normalized.length - 1).strip ();
        }

        while (normalized.has_suffix (".")) {
            normalized = normalized.substring (0, normalized.length - 1).strip ();
        }

        if (normalized.length == 0) {
            return "Cargando";
        }

        return normalized;
    }

    private void start_activity_animation () {
        stop_activity_animation ();
        activity_animation_frame = 0;
        update_activity_status_animation ();
        activity_animation_id = GLib.Timeout.add (320, () => {
            if (!request_in_flight) {
                activity_animation_id = 0;
                return GLib.Source.REMOVE;
            }

            activity_animation_frame = (activity_animation_frame + 1) % 4;
            update_activity_status_animation ();
            return GLib.Source.CONTINUE;
        });
    }

    private void stop_activity_animation () {
        if (activity_animation_id != 0) {
            GLib.Source.remove (activity_animation_id);
            activity_animation_id = 0;
        }
        activity_animation_frame = 0;
    }

    private void update_activity_status_animation () {
        string suffix = "";
        switch (activity_animation_frame) {
        case 1:
            suffix = ".";
            break;
        case 2:
            suffix = "..";
            break;
        case 3:
            suffix = "...";
            break;
        default:
            suffix = "";
            break;
        }

        activity_status_label.label = activity_phase_base + suffix;
    }

    private void on_reset_session_requested () {
        gateway.reset_session ();
        request_in_flight = false;
        query_entry.text = "";
        show_placeholder_result ("Sesi\u00f3n reiniciada. Haz una nueva pregunta\u2026");
        query_entry.grab_focus ();
    }

    private void on_mic_button_clicked () {
        if (request_in_flight) return;
        trigger_voice_input.begin ();
    }

    private async void trigger_voice_input () {
        set_controls_sensitive (false);
        request_in_flight = true;

        if (mic_button != null) {
            mic_button.get_style_context ().add_class ("sam-mic-recording");
        }
        show_activity_view ("Escuchando tu voz... Habla ahora.");

        try {
            string transcribed_text = yield gateway.listen_voice ();

            if (mic_button != null) {
                mic_button.get_style_context ().remove_class ("sam-mic-recording");
            }
            hide_activity_view ();

            if (transcribed_text.length > 0) {
                query_entry.text = transcribed_text;
                submit_query (transcribed_text);
            } else {
                set_controls_sensitive (true);
                request_in_flight = false;
                query_entry.grab_focus ();
            }
        } catch (GLib.Error e) {
            if (mic_button != null) {
                mic_button.get_style_context ().remove_class ("sam-mic-recording");
            }
            hide_activity_view ();
            set_controls_sensitive (true);
            request_in_flight = false;

            show_placeholder_result ("\u26a0 Error de voz: " + e.message);
            query_entry.grab_focus ();
        }
    }

    private void set_controls_sensitive (bool sensitive) {
        query_entry.sensitive = sensitive;
        send_button.sensitive = sensitive;
        reset_session_button.sensitive = sensitive;
        if (mic_button != null) {
            mic_button.sensitive = sensitive;
        }
    }

    private void clear_results () {
        foreach (var child in result_container.get_children ()) {
            result_container.remove (child);
        }
    }

    private void show_placeholder_result (string message) {
        clear_results ();

        var placeholder = new Gtk.Label (message) {
            halign          = Gtk.Align.START,
            wrap            = true,
            max_width_chars = 46
        };
        placeholder.get_style_context ().add_class ("sam-result");
        result_container.add (placeholder);
        result_container.show_all ();
        result_scroll.show ();
    }

    /* ------------------------------------------------------------------ */
    /*  Result rendering — parse file paths into clickable widgets          */
    /* ------------------------------------------------------------------ */

    private void populate_results (string text) {
        clear_results ();

        if (render_line_results (text)) {
            result_container.show_all ();
            return;
        }

        try {
            var regex = new GLib.Regex (
                "(?:"
                + "(?:~|/(?:home|tmp|root|var|usr|opt|etc|mnt|media))/[^\\s,;)\\]\"'<>`]+"
                + "|"
                + "(?:[A-Za-z0-9_.-]+/)+[^\\s,;)\\]\"'<>`]+"
                + ")"
            );

            GLib.MatchInfo match_info;
            int last_end = 0;
            string[] pending_pdfs = {};

            regex.match (text, 0, out match_info);

            while (match_info.matches ()) {
                int start_pos, end_pos;
                match_info.fetch_pos (0, out start_pos, out end_pos);

                if (start_pos > last_end) {
                    if (pending_pdfs.length > 0) {
                        add_pdf_bubbles (pending_pdfs);
                        pending_pdfs = {};
                    }
                    var segment = text.slice (last_end, start_pos).strip ();
                    if (segment.length > 0) {
                        add_text_widget (segment);
                    }
                }

                var raw_path = match_info.fetch (0);
                  while (raw_path.length > 0 &&
                      (raw_path.has_suffix (".") || raw_path.has_suffix (":") || raw_path.has_suffix ("`"))) {
                    raw_path = raw_path.slice (0, raw_path.length - 1);
                }

                if (raw_path.down ().has_suffix (".pdf")) {
                    pending_pdfs += raw_path;
                } else {
                    if (pending_pdfs.length > 0) {
                        add_pdf_bubbles (pending_pdfs);
                        pending_pdfs = {};
                    }
                    add_file_widget (raw_path);
                }

                last_end = end_pos;
                match_info.next ();
            }

            if (pending_pdfs.length > 0) {
                add_pdf_bubbles (pending_pdfs);
            }

            if (last_end < text.length) {
                var remaining = text.slice (last_end, text.length).strip ();
                if (remaining.length > 0) {
                    add_text_widget (remaining);
                }
            }

        } catch (GLib.RegexError e) {
            add_text_widget (text);
        }

        result_container.show_all ();
    }

    private bool render_line_results (string text) {
        bool rendered_any = false;
        string[] pending_pdfs = {};
        string pending_text = "";

        foreach (var raw_line in text.split ("\n")) {
            var line = raw_line.strip ();
            if (line.length == 0) {
                flush_pending_pdfs (ref pending_pdfs, ref rendered_any);
                flush_pending_text (ref pending_text, ref rendered_any);
                continue;
            }

            var path = extract_path_from_line (line);
            if (path != null) {
                flush_pending_text (ref pending_text, ref rendered_any);
                if (path.down ().has_suffix (".pdf")) {
                    pending_pdfs += path;
                } else {
                    flush_pending_pdfs (ref pending_pdfs, ref rendered_any);
                    add_file_widget ((owned) path);
                    rendered_any = true;
                }
                continue;
            }

            flush_pending_pdfs (ref pending_pdfs, ref rendered_any);
            if (pending_text.length > 0) {
                pending_text += "\n";
            }
            pending_text += line;
        }

        flush_pending_pdfs (ref pending_pdfs, ref rendered_any);
        flush_pending_text (ref pending_text, ref rendered_any);
        return rendered_any;
    }

    private void flush_pending_pdfs (ref string[] pending_pdfs, ref bool rendered_any) {
        if (pending_pdfs.length == 0) {
            return;
        }

        add_pdf_bubbles (pending_pdfs);
        pending_pdfs = {};
        rendered_any = true;
    }

    private void flush_pending_text (ref string pending_text, ref bool rendered_any) {
        var content = pending_text.strip ();
        if (content.length == 0) {
            pending_text = "";
            return;
        }

        add_text_widget (content);
        pending_text = "";
        rendered_any = true;
    }

    private string? extract_path_from_line (string line) {
        var candidate = line.strip ();

        try {
            var bullet_re = new GLib.Regex ("^(?:[-*\\u2022]|\\d+[.)])\\s+");
            candidate = bullet_re.replace (candidate, -1, 0, "").strip ();
        } catch (GLib.RegexError e) {
        }

        candidate = normalize_detected_path (candidate);
        if (!looks_like_standalone_path (candidate)) {
            return null;
        }

        return candidate;
    }

    private string normalize_detected_path (string path) {
        var normalized = path.strip ();

        while (normalized.length > 0 &&
            (normalized.has_prefix ("`") ||
             normalized.has_prefix ("\"") ||
             normalized.has_prefix ("'") ||
             normalized.has_prefix ("(") ||
             normalized.has_prefix ("[") ||
             normalized.has_prefix ("<"))) {
            normalized = normalized.substring (1).strip ();
        }

        while (normalized.length > 0 &&
            (normalized.has_suffix (".") ||
             normalized.has_suffix (":") ||
             normalized.has_suffix (";") ||
             normalized.has_suffix (",") ||
             normalized.has_suffix ("`") ||
             normalized.has_suffix ("\"") ||
             normalized.has_suffix ("'") ||
             normalized.has_suffix (")") ||
             normalized.has_suffix ("]") ||
             normalized.has_suffix (">"))) {
            normalized = normalized.slice (0, normalized.length - 1).strip ();
        }

        return normalized;
    }

    private bool looks_like_standalone_path (string candidate) {
        if (candidate.length == 0) {
            return false;
        }

        if (candidate.has_prefix ("/") || candidate.has_prefix ("~/")) {
            return true;
        }

        int slash_index = candidate.index_of ("/");
        if (slash_index <= 0) {
            return false;
        }

        var prefix = candidate.slice (0, slash_index);
        return prefix.index_of (" ") < 0 &&
            prefix.index_of ("\t") < 0 &&
            !prefix.has_prefix ("http:") &&
            !prefix.has_prefix ("https:");
    }

    private string resolve_result_path (string path) {
        var resolved = normalize_detected_path (path);
        if (resolved.has_prefix ("~")) {
            return GLib.Environment.get_home_dir () + resolved.substring (1);
        }
        if (!resolved.has_prefix ("/")) {
            return GLib.Path.build_filename (GLib.Environment.get_home_dir (), resolved);
        }
        return resolved;
    }

    private string filename_for_path (string resolved) {
        var parts = resolved.split ("/");
        var filename = parts[parts.length - 1];
        if (filename.length == 0 && parts.length > 1) {
            filename = parts[parts.length - 2] + "/";
        }
        return filename;
    }

    private void open_result_path (string path) {
        var resolved = resolve_result_path (path);
        var file = GLib.File.new_for_path (resolved);
        var file_uri = file.get_uri ();

        GLib.Idle.add (() => {
            try {
                if (!file.query_exists ()) {
                    warning ("panel-sam: file does not exist: %s", resolved);
                    return GLib.Source.REMOVE;
                }

                Gtk.show_uri_on_window (null, file_uri, Gdk.CURRENT_TIME);
            } catch (Error e) {
                warning ("panel-sam: could not open %s: %s", file_uri, e.message);
            }
            return GLib.Source.REMOVE;
        });
    }

    private void add_text_widget (string content) {
        var markup = markdown_to_pango (content);
        var label = new Gtk.Label (null) {
            halign          = Gtk.Align.START,
            wrap            = true,
            selectable      = true,
            use_markup      = true,
            max_width_chars = 46
        };
        label.set_markup (markup);
        label.get_style_context ().add_class ("sam-result");
        result_container.add (label);
    }

    /**
     * Lightweight Markdown \u2192 Pango markup converter.
     * Handles: ### headings, **bold**, *italic*, `code`, - list items.
     */
    private static string markdown_to_pango (string md) {
        var sb = new GLib.StringBuilder ();
        var lines = md.split ("\n");

        for (int i = 0; i < lines.length; i++) {
            var line = lines[i];
            string trimmed = line.strip ();

            if (trimmed.length == 0) {
                if (sb.len > 0) {
                    sb.append ("\n");
                }
                continue;
            }

            /* Headings: ### or ## or # */
            if (trimmed.has_prefix ("###")) {
                var heading = GLib.Markup.escape_text (trimmed.substring (3).strip ());
                sb.append ("<b>" + heading + "</b>\n");
                continue;
            }
            if (trimmed.has_prefix ("##")) {
                var heading = GLib.Markup.escape_text (trimmed.substring (2).strip ());
                sb.append ("<b>" + heading + "</b>\n");
                continue;
            }
            if (trimmed.has_prefix ("# ")) {
                var heading = GLib.Markup.escape_text (trimmed.substring (2).strip ());
                sb.append ("<b><big>" + heading + "</big></b>\n");
                continue;
            }

            /* List items: - or * at start */
            if (trimmed.has_prefix ("- ") || trimmed.has_prefix ("* ")) {
                var item_text = trimmed.substring (2);
                sb.append ("  \u2022 " + inline_md_to_pango (item_text) + "\n");
                continue;
            }

            /* Regular line */
            sb.append (inline_md_to_pango (trimmed) + "\n");
        }

        /* Trim trailing newlines */
        string result = sb.str;
        while (result.has_suffix ("\n")) {
            result = result.slice (0, result.length - 1);
        }
        return result;
    }

    /**
     * Convert inline markdown (**bold**, *italic*, `code`) to Pango markup.
     * Escapes any XML entities first.
     */
    private static string inline_md_to_pango (string text) {
        string escaped = GLib.Markup.escape_text (text);
        string result = escaped;

        /* **bold** \u2192 <b>bold</b> */
        try {
            var bold_re = new GLib.Regex ("\\*\\*(.+?)\\*\\*");
            result = bold_re.replace (result, -1, 0, "<b>\\1</b>");
        } catch (GLib.RegexError e) {}

        /* *italic* \u2192 <i>italic</i>  (but not inside <b> tags) */
        try {
            var italic_re = new GLib.Regex ("(?<!\\w)\\*(.+?)\\*(?!\\w)");
            result = italic_re.replace (result, -1, 0, "<i>\\1</i>");
        } catch (GLib.RegexError e) {}

        /* `code` \u2192 <tt>code</tt> */
        try {
            var code_re = new GLib.Regex ("`(.+?)`");
            result = code_re.replace (result, -1, 0, "<tt>\\1</tt>");
        } catch (GLib.RegexError e) {}

        return result;
    }

    private void add_file_widget (string path) {
        var resolved = resolve_result_path (path);
        var filename = filename_for_path (resolved);

        var is_dir = resolved.has_suffix ("/");
        var icon_name = is_dir ? "folder-symbolic" : "text-x-generic-symbolic";

        var icon = new Gtk.Image.from_icon_name (icon_name, Gtk.IconSize.MENU);
        icon.get_style_context ().add_class ("sam-file-icon");

        var name_lbl = new Gtk.Label (filename) {
            halign    = Gtk.Align.START,
            hexpand   = true,
            ellipsize = Pango.EllipsizeMode.MIDDLE
        };
        name_lbl.get_style_context ().add_class ("sam-file-name");

        var arrow = new Gtk.Label ("\u2197") {
            halign = Gtk.Align.END
        };
        arrow.get_style_context ().add_class ("sam-file-arrow");

        var row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 8);
        row.add (icon);
        row.add (name_lbl);
        row.add (arrow);

        var btn = new Gtk.Button () {
            relief = Gtk.ReliefStyle.NONE
        };
        btn.add (row);
        btn.get_style_context ().add_class ("sam-file-btn");
        btn.tooltip_text = resolved;
        btn.clicked.connect (() => {
            open_result_path (resolved);
        });

        result_container.add (btn);
    }

    /* PDF bubble row — horizontal pill buttons side by side */

    private void add_pdf_bubbles (string[] paths) {
        var row = new Gtk.FlowBox ();
        row.get_style_context ().add_class ("sam-pdf-row");
        row.selection_mode = Gtk.SelectionMode.NONE;
        row.activate_on_single_click = false;
        row.halign = Gtk.Align.START;
        row.max_children_per_line = 3;
        row.min_children_per_line = 1;
        row.set_column_spacing (6);
        row.set_row_spacing (6);

        foreach (var path in paths) {
            row.add (create_pdf_bubble (path));
        }

        result_container.add (row);
    }

    private Gtk.Widget create_pdf_bubble (string path) {
        var resolved = resolve_result_path (path);
        var filename = filename_for_path (resolved);

        var icon = new Gtk.Image.from_icon_name ("application-pdf-symbolic", Gtk.IconSize.MENU);
        icon.get_style_context ().add_class ("sam-pdf-bubble-icon");

        var name_lbl = new Gtk.Label (filename) {
            ellipsize = Pango.EllipsizeMode.MIDDLE,
            max_width_chars = 24
        };
        name_lbl.get_style_context ().add_class ("sam-pdf-bubble-name");

        var bubble_row = new Gtk.Box (Gtk.Orientation.HORIZONTAL, 5);
        bubble_row.add (icon);
        bubble_row.add (name_lbl);

        var btn = new Gtk.Button () {
            relief = Gtk.ReliefStyle.NONE
        };
        btn.add (bubble_row);
        btn.get_style_context ().add_class ("sam-pdf-bubble");
        btn.tooltip_text = resolved;
        btn.clicked.connect (() => {
            open_result_path (resolved);
        });

        return btn;
    }
}

/* ---------------------------------------------------------------------- */
/*  Module entry point — required by Wingpanel                             */
/* ---------------------------------------------------------------------- */

public Wingpanel.Indicator? get_indicator (Wingpanel.IndicatorManager.ServerType server_type) {
    if (server_type == Wingpanel.IndicatorManager.ServerType.GREETER) {
        return null;
    }

    return new Sam.Indicator ();
}
