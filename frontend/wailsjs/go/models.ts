export namespace cli {

	export class ConfigInfo {
	    path: string;
	    exists: boolean;
	    content: string;

	    static createFrom(source: any = {}) {
	        return new ConfigInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.exists = source["exists"];
	        this.content = source["content"];
	    }
	}
	export class FileStatus {
	    path: string;
	    status: string;

	    static createFrom(source: any = {}) {
	        return new FileStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	    }
	}
	export class IncomingFile {
	    path: string;
	    diff: string;

	    static createFrom(source: any = {}) {
	        return new IncomingFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.diff = source["diff"];
	    }
	}
	export class LinkStartInfo {
	    message: string;
	    incomingFiles: IncomingFile[];

	    static createFrom(source: any = {}) {
	        return new LinkStartInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.incomingFiles = this.convertValues(source["incomingFiles"], IncomingFile);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LinkedFile {
	    path: string;
	    error: string;

	    static createFrom(source: any = {}) {
	        return new LinkedFile(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.error = source["error"];
	    }
	}
	export class StatusInfo {
	    git_push_target: string;
	    local_dotfiles_dir: string;
	    branch: string;
	    last_commit: string;
	    last_sync: string;
	    files: FileStatus[];

	    static createFrom(source: any = {}) {
	        return new StatusInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.git_push_target = source["git_push_target"];
	        this.local_dotfiles_dir = source["local_dotfiles_dir"];
	        this.branch = source["branch"];
	        this.last_commit = source["last_commit"];
	        this.last_sync = source["last_sync"];
	        this.files = this.convertValues(source["files"], FileStatus);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
