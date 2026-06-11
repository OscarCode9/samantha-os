project(
  'io.elementary.panel.sam',
  'vala', 'c',
  version: '0.1.0',
  meson_version: '>= 1.0.0',
)

wingpanel_dep = dependency('wingpanel')

wingpanel_indicatorsdir = wingpanel_dep.get_variable(pkgconfig: 'indicatorsdir')

pkgdatadir = get_option('prefix') / get_option('datadir') / meson.project_name()

conf_data = configuration_data()
conf_data.set_quoted('PKGDATADIR', pkgdatadir)

config_vala = configure_file(
  input: 'src/Config.vala.in',
  output: 'Config.vala',
  configuration: conf_data,
)

dependencies = [
  wingpanel_dep,
  dependency('glib-2.0'),
  dependency('gobject-2.0'),
  dependency('gtk+-3.0'),
  dependency('libsoup-3.0'),
  dependency('json-glib-1.0'),
]

sources = files(
  'src/Indicator.vala',
  'src/Services/GatewayClient.vala',
)

shared_module(
  'sam',
  sources,
  config_vala,
  dependencies: dependencies,
  install: true,
  install_dir: wingpanel_indicatorsdir,
)

install_data(
  'data/styles/sam.css',
  install_dir: pkgdatadir / 'styles',
)

install_data(
  'data/sounds/chat-open.mp3',
  install_dir: pkgdatadir / 'sounds',
)
