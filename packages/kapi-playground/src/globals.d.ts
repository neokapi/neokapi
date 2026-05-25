// Allow side-effect CSS imports (the kit ships a plain stylesheet and pulls in
// xterm's CSS). The bundler handles these; TypeScript just needs the module to
// exist.
declare module "*.css";
