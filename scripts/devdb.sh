#!/bin/sh
# Throwaway Postgres on localhost:55432 (user postgres, password pg, database
# printdis) for tests and `make run`.
#
#   devdb.sh start   start it
#   devdb.sh stop    stop it and delete its data
#
# Runtime, first that works: podman, then docker (when its daemon answers),
# then local Postgres binaries (initdb/pg_ctl) with data in ./.pgdata.
# DEVDB=podman|docker|native forces one; PG_BIN=/path/to/bin picks the local
# binaries. Containers run Postgres 17 like CI; native uses whatever is
# installed, preferring Homebrew's postgresql@17.
set -eu

port=55432
name=printdis-pg
image=docker.io/library/postgres:17-alpine
data=.pgdata

pick_runtime() {
	if [ -n "${DEVDB:-}" ]; then
		echo "$DEVDB"
	elif command -v podman >/dev/null && podman info >/dev/null 2>&1; then
		echo podman
	elif command -v docker >/dev/null && docker info >/dev/null 2>&1; then
		echo docker
	else
		echo native
	fi
}

pg_bin() {
	if [ -n "${PG_BIN:-}" ]; then
		echo "$PG_BIN"
		return
	fi
	if command -v brew >/dev/null; then
		for f in postgresql@17 postgresql@18 postgresql; do
			p=$(brew --prefix "$f" 2>/dev/null) || continue
			if [ -x "$p/bin/pg_ctl" ]; then
				echo "$p/bin"
				return
			fi
		done
	fi
	if command -v pg_ctl >/dev/null; then
		dirname "$(command -v pg_ctl)"
		return
	fi
	echo "devdb: no podman or docker running and no Postgres binaries found;" >&2
	echo "devdb: install one (brew install postgresql@17, pacman -S postgresql) or set PG_BIN" >&2
	exit 1
}

start_native() {
	bin=$(pg_bin)
	if [ ! -d "$data" ]; then
		"$bin/initdb" -D "$data" -U postgres --auth=trust -E UTF8 --locale=C >/dev/null
	fi
	if "$bin/pg_ctl" -D "$data" status >/dev/null 2>&1; then
		echo "devdb: already running from $data"
		return
	fi
	# TCP only: no unix socket, so nothing needs /run/postgresql or /tmp.
	"$bin/pg_ctl" -D "$data" -l "$data/server.log" -w \
		-o "-p $port -c listen_addresses=localhost -c unix_socket_directories=" start >/dev/null
	"$bin/psql" -h localhost -p "$port" -U postgres -Atqc "select 1 from pg_database where datname = 'printdis'" |
		grep -q 1 || "$bin/createdb" -h localhost -p "$port" -U postgres printdis
	echo "devdb: $("$bin/postgres" --version) from $bin on :$port, data in $data"
}

start_container() {
	if [ -n "$("$1" ps -q --filter "name=^$name\$")" ]; then
		echo "devdb: already running in $1"
		return
	fi
	"$1" run -d --rm --name "$name" -e POSTGRES_PASSWORD=pg -e POSTGRES_DB=printdis \
		-p "$port:5432" "$image" >/dev/null
	i=0
	until "$1" exec "$name" pg_isready -U postgres -d printdis -h localhost >/dev/null 2>&1; do
		i=$((i + 1))
		if [ "$i" -ge 60 ]; then
			echo "devdb: Postgres in $1 did not become ready" >&2
			exit 1
		fi
		sleep 0.5
	done
	echo "devdb: $image in $1 on :$port"
}

stop() {
	if [ -d "$data" ]; then
		bin=$(pg_bin)
		"$bin/pg_ctl" -D "$data" -m fast stop >/dev/null 2>&1 || true
		rm -rf "$data"
		echo "devdb: stopped native Postgres, removed $data"
		return
	fi
	for rt in podman docker; do
		if command -v "$rt" >/dev/null && "$rt" stop "$name" >/dev/null 2>&1; then
			echo "devdb: stopped $name in $rt"
			return
		fi
	done
	echo "devdb: nothing running"
}

case "${1:-}" in
start)
	rt=$(pick_runtime)
	case "$rt" in
	podman | docker) start_container "$rt" ;;
	native) start_native ;;
	*)
		echo "devdb: DEVDB must be podman, docker or native" >&2
		exit 2
		;;
	esac
	;;
stop) stop ;;
*)
	echo "usage: $0 start|stop" >&2
	exit 2
	;;
esac
