# Translating

The program is in English, with a built-in Hungarian translation. Other
languages can be added with a plain text file, without building the program.

1. *Language > Make a translation file.* The program writes a UTF-8 text file
   with every English text on an `EN:` line and an empty `TR:` line under it.
2. Set the language name on the `@name =` line and fill in the `TR:` lines.
   Keep the `%s`, `%d` and similar placeholders, in the same order; `\n` is a
   line break. An empty `TR:` line leaves that text in English.
3. Name the file after the language code, for example `de.lng`, and put it
   into the `lang` folder (*Language > Open language folder*).
4. *Language > Reload language files*, then choose the language.

New languages are welcome: open a pull request or an issue with the `.lng`
file.
