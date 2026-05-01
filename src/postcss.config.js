/*
  PostCSS is a JavaScript tool that transforms CSS using plugins.

  JavaScript concept: `module.exports`
  - In CommonJS (Node's classic module system), a file "exports" values by
    assigning to `module.exports`.
  - Other files can then `require(...)` this config.

  Project concept:
  - Our CSS pipeline runs Tailwind first (generate utility CSS from classes
    found in templates), then Autoprefixer (add vendor prefixes where needed).
*/
module.exports = {
  plugins: {
    /*
      Plugin object shape:
      - key   => plugin package name
      - value => plugin options object

      Empty object `{}` means "use default options".
    */
    tailwindcss: {},
    autoprefixer: {}
  }
};
