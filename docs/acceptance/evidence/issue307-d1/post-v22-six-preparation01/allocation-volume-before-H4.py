def volume(self,name):
        value=docker('volume','create','--label','lifecycle307.remaining='+EXECUTION,name).decode('utf-8','strict').strip()
        need(value==name,'new volume identity differs');self.resources.append({'volume':name});return name
