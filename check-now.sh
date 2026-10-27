# Source from a POSIX shell startup file.
case $- in *i*) command -v check-now >/dev/null 2>&1 && check-now shell 2>/dev/null ;; esac
