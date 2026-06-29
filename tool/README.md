# Local Tool Directory

This directory is for optional local build tools used when the same tool is not available from the system PATH.

The release scripts resolve tools in this order:

1. Explicit environment variable, when one exists.
2. System PATH.
3. This project-local `tool` directory.
4. Tool-specific legacy install locations, only where useful.

Expected optional layout:

```text
tool/
  go/bin/go.exe
  go1.20.14/go/bin/go.exe
  gradle-8.9/bin/gradle.bat
  gradle/bin/gradle.bat
  inno/ISCC.exe
  Inno Setup 6/ISCC.exe
  signtool/signtool.exe
```

`tool/signtool` must include the Windows SDK signing dependency DLLs and manifests, not only `signtool.exe`.

The actual tool binaries are intentionally ignored by git.
