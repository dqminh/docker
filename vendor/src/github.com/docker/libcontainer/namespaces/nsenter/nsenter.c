// +build cgo
//
// formated with indent -linux nsenter.c

#include <errno.h>
#include <fcntl.h>
#include <linux/limits.h>
#include <linux/sched.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <sys/types.h>
#include <unistd.h>
#include <getopt.h>

static const kBufSize = 256;
static const char *kNsEnter = "nsenter";
pid_t child;

// forward the received signal to child
void forward_signal(int sig)
{
	if (child > 0) {
		kill(child, sig);
	}
}

void get_args(int *argc, char ***argv)
{
	// Read argv
	int fd = open("/proc/self/cmdline", O_RDONLY);

	// Read the whole commandline.
	ssize_t contents_size = 0;
	ssize_t contents_offset = 0;
	char *contents = NULL;
	ssize_t bytes_read = 0;
	do {
		contents_size += kBufSize;
		contents = (char *)realloc(contents, contents_size);
		bytes_read =
			read(fd, contents + contents_offset,
			     contents_size - contents_offset);
		contents_offset += bytes_read;
	}
	while (bytes_read > 0);
	close(fd);

	// Parse the commandline into an argv. /proc/self/cmdline has \0 delimited args.
	ssize_t i;
	*argc = 0;
	for (i = 0; i < contents_offset; i++) {
		if (contents[i] == '\0') {
			(*argc)++;
		}
	}
	*argv = (char **)malloc(sizeof(char *) * ((*argc) + 1));
	int idx;
	for (idx = 0; idx < (*argc); idx++) {
		(*argv)[idx] = contents;
		contents += strlen(contents) + 1;
	}
	(*argv)[*argc] = NULL;
}

// Use raw setns syscall for versions of glibc that don't include it (namely glibc-2.12)
#if __GLIBC__ == 2 && __GLIBC_MINOR__ < 14
#define _GNU_SOURCE
#include <sched.h>
#include "syscall.h"
#ifdef SYS_setns
int setns(int fd, int nstype)
{
	return syscall(SYS_setns, fd, nstype);
}
#endif
#endif

void print_usage()
{
	fprintf(stderr,
		"nsenter --nspid <pid> --console <console> -- cmd1 arg1 arg2...\n");
}

void nsenter()
{
	int argc, c;
	char **argv;
	get_args(&argc, &argv);

	// check argv 0 to ensure that we are supposed to setns
	// we use strncmp to test for a value of "nsenter" but also allows alternate implmentations
	// after the setns code path to continue to use the argv 0 to determine actions to be run
	// resulting in the ability to specify "nsenter-mknod", "nsenter-exec", etc...
	if (strncmp(argv[0], kNsEnter, strlen(kNsEnter)) != 0) {
		return;
	}

	static const struct option longopts[] = {
		{"nspid", required_argument, NULL, 'n'},
		{"console", required_argument, NULL, 't'},
		{NULL, 0, NULL, 0}
	};
    
	pid_t init_pid = -1;
	char *init_pid_str = NULL;
	char *console = NULL;
	while ((c = getopt_long_only(argc, argv, "n:c:", longopts, NULL)) != -1) {
		switch (c) {
		case 'n':
			init_pid_str = optarg;
			break;
		case 't':
			console = optarg;
			break;
		}
	}

	if (init_pid_str == NULL) {
		print_usage();
		exit(1);
	}

	init_pid = strtol(init_pid_str, NULL, 10);
	if ((init_pid == 0 && errno == EINVAL) || errno == ERANGE) {
		fprintf(stderr,
			"nsenter: Failed to parse PID from \"%s\" with output \"%d\" and error: \"%s\"\n",
			init_pid_str, init_pid, strerror(errno));
		print_usage();
		exit(1);
	}

	argc -= 3;
	argv += 3;

	if (setsid() == -1) {
		fprintf(stderr, "setsid failed. Error: %s\n", strerror(errno));
		exit(1);
	}
	// before we setns we need to dup the console
	int consolefd = -1;
	if (console != NULL) {
		consolefd = open(console, O_RDWR);
		if (consolefd < 0) {
			fprintf(stderr,
				"nsenter: failed to open console %s %s\n",
				console, strerror(errno));
			exit(1);
		}
	}
	// Setns on all supported namespaces.
	char ns_dir[PATH_MAX];
	memset(ns_dir, 0, PATH_MAX);
	snprintf(ns_dir, PATH_MAX - 1, "/proc/%d/ns/", init_pid);

	char *namespaces[] = { "ipc", "uts", "net", "pid", "mnt" };
	const int num = sizeof(namespaces) / sizeof(char *);
	int i;
	for (i = 0; i < num; i++) {
		char buf[PATH_MAX];
		memset(buf, 0, PATH_MAX);
		snprintf(buf, PATH_MAX - 1, "%s%s", ns_dir, namespaces[i]);
		int fd = open(buf, O_RDONLY);
		if (fd == -1) {
			// Ignore nonexistent namespaces.
			if (errno == ENOENT)
				continue;

			fprintf(stderr,
				"nsenter: Failed to open ns file \"%s\" for ns \"%s\" with error: \"%s\"\n",
				buf, namespaces[i], strerror(errno));
			exit(1);
		}
		// Set the namespace.
		if (setns(fd, 0) == -1) {
			fprintf(stderr,
				"nsenter: Failed to setns for \"%s\" with error: \"%s\"\n",
				namespaces[i], strerror(errno));
			exit(1);
		}
		close(fd);
	}

	// We must fork to actually enter the PID namespace.
	child = fork();
	if (child == 0) {
		if (consolefd != -1) {
			if (dup2(consolefd, STDIN_FILENO) != 0) {
				fprintf(stderr, "nsenter: failed to dup 0 %s\n",
					strerror(errno));
				exit(1);
			}
			if (dup2(consolefd, STDOUT_FILENO) != STDOUT_FILENO) {
				fprintf(stderr, "nsenter: failed to dup 1 %s\n",
					strerror(errno));
				exit(1);
			}
			if (dup2(consolefd, STDERR_FILENO) != STDERR_FILENO) {
				fprintf(stderr, "nsenter: failed to dup 2 %s\n",
					strerror(errno));
				exit(1);
			}
		}
		if (prctl(PR_SET_PDEATHSIG, SIGKILL, 0, 0, 0) == -1) {
			fprintf(stderr,
				"nsenter: failed to set parent death signal: %s\n",
				strerror(errno));
			exit(1);
		}
		// Finish executing, let the Go runtime take over.
		return;
	} else {
		// forward all received signal to child
		// list of signals taken docker/pkg/signal
		signal(SIGABRT, forward_signal);
		signal(SIGALRM, forward_signal);
		signal(SIGBUS, forward_signal);
		signal(SIGCHLD, forward_signal);
		signal(SIGCLD, forward_signal);
		signal(SIGCONT, forward_signal);
		signal(SIGFPE, forward_signal);
		signal(SIGHUP, forward_signal);
		signal(SIGILL, forward_signal);
		signal(SIGINT, forward_signal);
		signal(SIGIO, forward_signal);
		signal(SIGIOT, forward_signal);
		signal(SIGKILL, forward_signal);
		signal(SIGPIPE, forward_signal);
		signal(SIGPOLL, forward_signal);
		signal(SIGPROF, forward_signal);
		signal(SIGPWR, forward_signal);
		signal(SIGQUIT, forward_signal);
		signal(SIGSEGV, forward_signal);
		signal(SIGSTKFLT, forward_signal);
		signal(SIGSTOP, forward_signal);
		signal(SIGSYS, forward_signal);
		signal(SIGTERM, forward_signal);
		signal(SIGTRAP, forward_signal);
		signal(SIGTSTP, forward_signal);
		signal(SIGTTIN, forward_signal);
		signal(SIGTTOU, forward_signal);
		signal(SIGUNUSED, forward_signal);
		signal(SIGURG, forward_signal);
		signal(SIGUSR1, forward_signal);
		signal(SIGUSR2, forward_signal);
		signal(SIGVTALRM, forward_signal);
		signal(SIGWINCH, forward_signal);
		signal(SIGXCPU, forward_signal);
		signal(SIGXFSZ, forward_signal);

		// Parent, wait for the child.
		int status = 0;
		if (waitpid(child, &status, 0) == -1) {
			fprintf(stderr,
				"nsenter: Failed to waitpid with error: \"%s\"\n",
				strerror(errno));
			exit(1);
		}
		// Forward the child's exit code or re-send its death signal.
		if (WIFEXITED(status)) {
			exit(WEXITSTATUS(status));
		} else if (WIFSIGNALED(status)) {
			kill(getpid(), WTERMSIG(status));
		}

		exit(1);
	}

	return;
}
