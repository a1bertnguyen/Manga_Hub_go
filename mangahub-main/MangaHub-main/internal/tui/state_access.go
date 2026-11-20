package tui

// Connection flags are shared by network workers and the UI event loop.
func (s *AppState) setConnection(protocol string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch protocol {
	case "tcp":
		s.TCPConnected = active
		if !active {
			s.TCPSessionID = ""
		}
	case "udp":
		s.UDPSubscribed = active
	case "ws":
		s.WSConnected = active
	}
}

func (s *AppState) connected(protocol string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch protocol {
	case "tcp":
		return s.TCPConnected
	case "udp":
		return s.UDPSubscribed
	case "ws":
		return s.WSConnected
	}
	return false
}

func (s *AppState) setSession(token, id, username string) {
	s.mu.Lock()
	s.Token, s.UserID, s.Username = token, id, username
	s.TCPSessionID, s.TCPConnected = "", false
	s.mu.Unlock()
}

func (s *AppState) tcpSessionID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TCPSessionID
}

func (s *AppState) token() string    { s.mu.RLock(); defer s.mu.RUnlock(); return s.Token }
func (s *AppState) userID() string   { s.mu.RLock(); defer s.mu.RUnlock(); return s.UserID }
func (s *AppState) username() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.Username }
